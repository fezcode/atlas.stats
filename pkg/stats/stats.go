package stats

import (
	"context"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// HistorySize is the number of samples retained for sparklines.
const HistorySize = 60

type ProcessInfo struct {
	PID      int32
	Name     string
	CPU      float64 // % of total system CPU (summed across cores)
	Mem      uint64  // RSS bytes
	DiskIO   uint64  // cumulative read+write bytes
	DiskRate uint64  // bytes / sample-interval
	NetConns int
}

type DiskInfo struct {
	Path        string
	Total       uint64
	Used        uint64
	Free        uint64
	UsedPercent float64
}

// Snapshot is an immutable copy of the most recent collection.
type Snapshot struct {
	Hostname string
	OS       string
	Platform string
	KernelV  string
	Uptime   uint64

	NumCPU      int
	CPUUsage    float64 // % of total CPU, 0..100
	MemoryTotal uint64
	MemoryUsed  uint64
	MemoryFree  uint64

	Disks []DiskInfo

	NetSent uint64
	NetRecv uint64
	NetRate uint64 // combined bytes/sec

	CPUHistory []float64
	MemHistory []float64
	NetHistory []float64

	TopCPU  []ProcessInfo
	TopMem  []ProcessInfo
	TopDisk []ProcessInfo
	TopNet  []ProcessInfo

	Generated time.Time
	Ready     bool
}

type procState struct {
	proc        *process.Process
	name        string
	lastTotal   float64   // cpu.TimesStat user+system
	lastSampled time.Time // wallclock
	lastDiskIO  uint64
	seen        bool
}

type Monitor struct {
	mu   sync.RWMutex
	snap Snapshot

	procs map[int32]*procState

	lastNet  uint64
	lastNetT time.Time

	cpuHist *ring
	memHist *ring
	netHist *ring

	// Cached slow-changing data.
	hostCache *host.InfoStat
	diskCache []DiskInfo
	slowAt    time.Time

	numCPU int
}

func NewMonitor() *Monitor {
	return &Monitor{
		procs:   make(map[int32]*procState),
		cpuHist: newRing(HistorySize),
		memHist: newRing(HistorySize),
		netHist: newRing(HistorySize),
		numCPU:  runtime.NumCPU(),
	}
}

// Run performs stats collection on its own goroutine until ctx is canceled.
// Only one collection runs at a time — callers read via Snapshot().
func (m *Monitor) Run(ctx context.Context, interval time.Duration) {
	// Prime the non-blocking cpu.Percent call; first result is a delta.
	_, _ = cpu.Percent(0, false)
	m.collect()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.collect()
		}
	}
}

// Snapshot returns a copy of the most recent collection.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snap
}

func (m *Monitor) collect() {
	now := time.Now()
	var s Snapshot
	s.Generated = now
	s.NumCPU = m.numCPU

	// --- Slow-changing: host + disks, every 10s ---
	if m.hostCache == nil || now.Sub(m.slowAt) > 10*time.Second {
		if h, err := host.Info(); err == nil {
			m.hostCache = h
		}
		m.diskCache = collectDisks()
		m.slowAt = now
	}
	if m.hostCache != nil {
		s.Hostname = m.hostCache.Hostname
		s.OS = m.hostCache.OS
		s.Platform = m.hostCache.Platform
		s.KernelV = m.hostCache.KernelVersion
		s.Uptime = m.hostCache.Uptime
	}
	s.Disks = m.diskCache

	// --- CPU (non-blocking, returns delta since previous call) ---
	if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 {
		s.CPUUsage = p[0]
	}

	// --- Memory ---
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemoryTotal = vm.Total
		s.MemoryUsed = vm.Used
		s.MemoryFree = vm.Free
	}

	// --- Network counters + rate ---
	if n, err := net.IOCounters(false); err == nil && len(n) > 0 {
		s.NetSent = n[0].BytesSent
		s.NetRecv = n[0].BytesRecv
		cur := s.NetSent + s.NetRecv
		if !m.lastNetT.IsZero() && cur >= m.lastNet {
			dt := now.Sub(m.lastNetT).Seconds()
			if dt > 0 {
				s.NetRate = uint64(float64(cur-m.lastNet) / dt)
			}
		}
		m.lastNet, m.lastNetT = cur, now
	}

	// --- Aggregate connections ONCE — the expensive call. ---
	connByPID := make(map[int32]int)
	if conns, err := net.Connections("inet"); err == nil {
		for _, c := range conns {
			if c.Pid != 0 {
				connByPID[c.Pid]++
			}
		}
	}

	// --- Per-process ---
	pids, _ := process.Pids()
	procInfos := make([]ProcessInfo, 0, len(pids))
	for _, ps := range m.procs {
		ps.seen = false
	}

	for _, pid := range pids {
		st := m.procs[pid]
		if st == nil {
			p, err := process.NewProcess(pid)
			if err != nil {
				continue
			}
			name, _ := p.Name()
			st = &procState{proc: p, name: name}
			m.procs[pid] = st
		}
		st.seen = true

		var cpuPct float64
		if t, err := st.proc.Times(); err == nil {
			total := t.User + t.System
			if !st.lastSampled.IsZero() {
				wall := now.Sub(st.lastSampled).Seconds()
				delta := total - st.lastTotal
				if wall > 0 && delta >= 0 {
					// % of a single core, then normalize by core count so
					// the sum tracks the system CPU gauge.
					cpuPct = 100.0 * delta / wall / float64(m.numCPU)
				}
			}
			st.lastTotal = total
			st.lastSampled = now
		}

		var memUsage uint64
		if mi, _ := st.proc.MemoryInfo(); mi != nil {
			memUsage = mi.RSS
		}

		var diskIO, diskRate uint64
		if io, _ := st.proc.IOCounters(); io != nil {
			diskIO = io.ReadBytes + io.WriteBytes
			if st.lastDiskIO > 0 && diskIO >= st.lastDiskIO {
				diskRate = diskIO - st.lastDiskIO
			}
		}
		st.lastDiskIO = diskIO

		procInfos = append(procInfos, ProcessInfo{
			PID:      pid,
			Name:     st.name,
			CPU:      cpuPct,
			Mem:      memUsage,
			DiskIO:   diskIO,
			DiskRate: diskRate,
			NetConns: connByPID[pid],
		})
	}
	for pid, ps := range m.procs {
		if !ps.seen {
			delete(m.procs, pid)
		}
	}

	s.TopCPU = topN(procInfos, 6, func(a, b ProcessInfo) bool {
		if a.CPU != b.CPU {
			return a.CPU > b.CPU
		}
		return a.PID < b.PID
	})
	s.TopMem = topN(procInfos, 6, func(a, b ProcessInfo) bool {
		if a.Mem != b.Mem {
			return a.Mem > b.Mem
		}
		return a.PID < b.PID
	})
	s.TopDisk = topN(procInfos, 6, func(a, b ProcessInfo) bool {
		if a.DiskRate != b.DiskRate {
			return a.DiskRate > b.DiskRate
		}
		return a.PID < b.PID
	})
	s.TopNet = topN(procInfos, 6, func(a, b ProcessInfo) bool {
		if a.NetConns != b.NetConns {
			return a.NetConns > b.NetConns
		}
		return a.PID < b.PID
	})

	// History
	m.cpuHist.push(s.CPUUsage)
	if s.MemoryTotal > 0 {
		m.memHist.push(100.0 * float64(s.MemoryUsed) / float64(s.MemoryTotal))
	} else {
		m.memHist.push(0)
	}
	m.netHist.push(float64(s.NetRate))

	s.CPUHistory = m.cpuHist.values()
	s.MemHistory = m.memHist.values()
	s.NetHistory = m.netHist.values()
	s.Ready = true

	m.mu.Lock()
	m.snap = s
	m.mu.Unlock()
}

func collectDisks() []DiskInfo {
	parts, err := disk.Partitions(false)
	if err != nil {
		return nil
	}
	var out []DiskInfo
	for _, p := range parts {
		if strings.HasPrefix(p.Device, "/dev/loop") ||
			strings.HasPrefix(p.Mountpoint, "/snap") ||
			strings.HasPrefix(p.Mountpoint, "/var/lib/docker") {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		out = append(out, DiskInfo{
			Path:        p.Mountpoint,
			Total:       u.Total,
			Used:        u.Used,
			Free:        u.Free,
			UsedPercent: u.UsedPercent,
		})
	}
	return out
}

func topN(ps []ProcessInfo, n int, less func(a, b ProcessInfo) bool) []ProcessInfo {
	if len(ps) == 0 {
		return nil
	}
	out := make([]ProcessInfo, len(ps))
	copy(out, ps)
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// --- ring buffer ------------------------------------------------------------

type ring struct {
	data []float64
	idx  int
	full bool
}

func newRing(n int) *ring { return &ring{data: make([]float64, n)} }

func (r *ring) push(v float64) {
	r.data[r.idx] = v
	r.idx = (r.idx + 1) % len(r.data)
	if r.idx == 0 {
		r.full = true
	}
}

func (r *ring) values() []float64 {
	out := make([]float64, 0, len(r.data))
	if r.full {
		out = append(out, r.data[r.idx:]...)
	}
	out = append(out, r.data[:r.idx]...)
	return out
}
