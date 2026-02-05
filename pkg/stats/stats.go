package stats

import (
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type ProcessInfo struct {
	PID      int32
	Name     string
	CPU      float64
	Mem      uint64
	DiskIO   uint64 // Cumulative
	DiskRate uint64 // Bytes per second
	NetConns int
}

type DiskInfo struct {
	Path        string
	Total       uint64
	Used        uint64
	Free        uint64
	UsedPercent float64
}

type SystemStats struct {
	CPUUsage    float64
	MemoryTotal uint64
	MemoryUsed  uint64
	MemoryFree  uint64

	Disks []DiskInfo

	Uptime   uint64
	Hostname string
	OS       string
	Platform string
	NetSent  uint64
	NetRecv  uint64

	TopCPU  []ProcessInfo
	TopMem  []ProcessInfo
	TopDisk []ProcessInfo
	TopNet  []ProcessInfo
}

type Monitor struct {
	procs     map[int32]*process.Process
	lastStats map[int32]ProcessInfo
	lastTime  time.Time
}

func NewMonitor() *Monitor {
	return &Monitor{
		procs:     make(map[int32]*process.Process),
		lastStats: make(map[int32]ProcessInfo),
		lastTime:  time.Now(),
	}
}

func (m *Monitor) GetStats() (SystemStats, error) {
	var s SystemStats

	// System CPU
	cpuPercentages, err := cpu.Percent(200*time.Millisecond, false)
	if err == nil && len(cpuPercentages) > 0 {
		s.CPUUsage = cpuPercentages[0]
	}

	// Memory
	vm, err := mem.VirtualMemory()
	if err == nil {
		s.MemoryTotal = vm.Total
		s.MemoryUsed = vm.Used
		s.MemoryFree = vm.Free
	}

	// Disks
	parts, err := disk.Partitions(false)
	if err == nil {
		for _, p := range parts {
			// Filter loop devices, snaps, etc if needed.
			// On Windows, 'false' gives physical drives mostly.
			// On Linux, we might get /dev/loop.
			if strings.HasPrefix(p.Device, "/dev/loop") {
				continue
			}

			u, err := disk.Usage(p.Mountpoint)
			if err == nil {
				s.Disks = append(s.Disks, DiskInfo{
					Path:        p.Mountpoint,
					Total:       u.Total,
					Used:        u.Used,
					Free:        u.Free,
					UsedPercent: u.UsedPercent,
				})
			}
		}
	}

	// Host
	h, err := host.Info()
	if err == nil {
		s.Uptime = h.Uptime
		s.Hostname = h.Hostname
		s.OS = h.OS
		s.Platform = h.Platform
	}

	// Net (System wide)
	n, err := net.IOCounters(false)
	if err == nil && len(n) > 0 {
		s.NetSent = n[0].BytesSent
		s.NetRecv = n[0].BytesRecv
	}

	// Processes
	pids, err := process.Pids()
	if err == nil {
		var procInfos []ProcessInfo
		now := time.Now()
		duration := now.Sub(m.lastTime).Seconds()
		if duration <= 0 {
			duration = 1
		}

		for _, pid := range pids {
			var proc *process.Process
			if existing, ok := m.procs[pid]; ok {
				proc = existing
			} else {
				newProc, err := process.NewProcess(pid)
				if err != nil {
					continue
				}
				proc = newProc
				m.procs[pid] = proc
			}

			name, _ := proc.Name()

			cpuVal, err := proc.CPUPercent()
			if err != nil {
				cpuVal = 0
			}

			memInfo, _ := proc.MemoryInfo()
			memUsage := uint64(0)
			if memInfo != nil {
				memUsage = memInfo.RSS
			}

			io, _ := proc.IOCounters()
			diskIO := uint64(0)
			if io != nil {
				diskIO = io.ReadBytes + io.WriteBytes
			}

			diskRate := uint64(0)
			if last, ok := m.lastStats[pid]; ok {
				if diskIO >= last.DiskIO {
					diskRate = uint64(float64(diskIO-last.DiskIO) / duration)
				}
			}

			conns, _ := proc.Connections()
			netConns := len(conns)

			info := ProcessInfo{
				PID:      pid,
				Name:     name,
				CPU:      cpuVal,
				Mem:      memUsage,
				DiskIO:   diskIO,
				DiskRate: diskRate,
				NetConns: netConns,
			}
			procInfos = append(procInfos, info)
			m.lastStats[pid] = info
		}

		m.lastTime = now

		// Cleanup dead processes
		activePids := make(map[int32]bool)
		for _, pid := range pids {
			activePids[pid] = true
		}
		for pid := range m.procs {
			if !activePids[pid] {
				delete(m.procs, pid)
				delete(m.lastStats, pid)
			}
		}

		// Sort and slice
		s.TopCPU = getTop(procInfos, func(p1, p2 ProcessInfo) bool {
			if p1.CPU != p2.CPU {
				return p1.CPU > p2.CPU
			}
			return p1.PID < p2.PID
		}, 5)

		s.TopMem = getTop(procInfos, func(p1, p2 ProcessInfo) bool {
			if p1.Mem != p2.Mem {
				return p1.Mem > p2.Mem
			}
			return p1.PID < p2.PID
		}, 5)

		s.TopDisk = getTop(procInfos, func(p1, p2 ProcessInfo) bool {
			if p1.DiskRate != p2.DiskRate {
				return p1.DiskRate > p2.DiskRate
			}
			return p1.PID < p2.PID
		}, 5)

		s.TopNet = getTop(procInfos, func(p1, p2 ProcessInfo) bool {
			if p1.NetConns != p2.NetConns {
				return p1.NetConns > p2.NetConns
			}
			return p1.PID < p2.PID
		}, 5)
	}

	return s, nil
}

func getTop(procs []ProcessInfo, less func(p1, p2 ProcessInfo) bool, limit int) []ProcessInfo {
	if len(procs) == 0 {
		return nil
	}
	sorted := make([]ProcessInfo, len(procs))
	copy(sorted, procs)
	sort.SliceStable(sorted, func(i, j int) bool {
		return less(sorted[i], sorted[j])
	})
	if len(sorted) > limit {
		return sorted[:limit]
	}
	return sorted
}
