package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"atlas.stats/pkg/stats"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time
type snapMsg stats.Snapshot

type model struct {
	monitor *stats.Monitor
	version string
	snap    stats.Snapshot

	width, height int
	scroll        int
	paused        bool
	blink         bool
	frame         int
	started       time.Time
}

func newModel(m *stats.Monitor, version string) model {
	return model{monitor: m, version: version, started: time.Now()}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(heartbeat(), m.pullSnap())
}

func heartbeat() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) pullSnap() tea.Cmd {
	return func() tea.Msg { return snapMsg(m.monitor.Snapshot()) }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "p", " ":
			m.paused = !m.paused
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		case "g", "home":
			m.scroll = 0
		case "G", "end":
			m.scroll = 9999
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll = 0
		return m, tea.Batch(tea.ClearScreen, m.pullSnap())
	case tickMsg:
		m.frame++
		m.blink = m.frame%2 == 0
		if m.paused {
			return m, heartbeat()
		}
		return m, tea.Batch(heartbeat(), m.pullSnap())
	case snapMsg:
		m.snap = stats.Snapshot(msg)
	}
	return m, nil
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.width < 64 {
		return sCrit.Render(" terminal too narrow — resize to ≥ 64 columns ")
	}

	var blocks []string
	blocks = append(blocks, m.renderMasthead())

	twoCol := m.width >= 100
	gutter := 1
	if twoCol {
		halfW := (m.width - gutter) / 2
		otherW := m.width - gutter - halfW

		kernel := m.renderKernel(halfW)
		network := m.renderNetwork(otherW)
		blocks = append(blocks, joinH(gutter, kernel, network))

		cpu := m.renderCPU(halfW)
		memB := m.renderMemory(otherW)
		blocks = append(blocks, joinH(gutter, cpu, memB))
	} else {
		blocks = append(blocks,
			m.renderKernel(m.width),
			m.renderNetwork(m.width),
			m.renderCPU(m.width),
			m.renderMemory(m.width),
		)
	}
	blocks = append(blocks,
		m.renderStorage(m.width),
		m.renderTopProcs(m.width),
	)

	body := strings.Join(blocks, "\n")

	full := body + "\n" + m.renderFooter()

	// Apply scroll window. Always pad to height so shrinking the terminal
	// doesn't leave ghosts from the previous (taller) paint.
	lines := strings.Split(full, "\n")
	available := m.height
	if len(lines) <= available {
		blank := strings.Repeat(" ", m.width)
		for len(lines) < available {
			lines = append(lines, blank)
		}
		return strings.Join(lines, "\n")
	}
	maxOff := len(lines) - available
	if m.scroll > maxOff {
		m.scroll = maxOff
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	return strings.Join(lines[m.scroll:m.scroll+available], "\n")
}

// --- Masthead ---------------------------------------------------------------

func (m model) renderMasthead() string {
	s := m.snap
	w := m.width

	rule := sBorder.Render(strings.Repeat("━", w))

	// Title: spaced caps
	title := sMastTitle.Render("A T L A S") +
		sDim.Render("  ·  ") +
		sMastTitle.Render("T E L E M E T R Y")

	// Right side: clock · REC/PAUSED · version
	clock := sMastClock.Render(time.Now().Format("15:04:05"))
	var rec string
	switch {
	case m.paused:
		rec = sHot.Render("◼ PAUSED")
	case m.blink:
		rec = sRec.Render("● REC")
	default:
		rec = sDim.Render("● REC")
	}
	ver := sDim.Render("v" + m.version)
	right := horiz(clock, rec, ver)

	// Line 1: title left, right meta right-justified.
	titleW := lipgloss.Width(title)
	rightW := lipgloss.Width(right)
	pad := w - 2 - titleW - rightW
	if pad < 1 {
		pad = 1
	}
	line1 := "  " + title + strings.Repeat(" ", pad) + right

	// Line 2: meta
	host := sDim.Render("HOST ") + sValue.Render(nonempty(s.Hostname, "—"))
	plat := sDim.Render("PLATFORM ") + sValue.Render(nonempty(s.Platform, s.OS))
	up := sDim.Render("UPTIME ") + sValue.Render(formatUptime(s.Uptime))
	cores := sDim.Render("CORES ") + sValue.Render(fmt.Sprintf("%d", s.NumCPU))
	line2 := "  " + horiz(host, plat, cores, up)
	// Trim if too long.
	if lipgloss.Width(line2) > w {
		line2 = "  " + horiz(host, plat, up)
	}
	if lipgloss.Width(line2) > w {
		line2 = "  " + horiz(host, up)
	}

	return strings.Join([]string{rule, line1, line2, rule}, "\n")
}

// --- Sections ---------------------------------------------------------------

func (m model) renderKernel(w int) string {
	s := m.snap
	col := 11
	body := strings.Join([]string{
		labelValue("HOST", sValue.Render(nonempty(s.Hostname, "—")), col),
		labelValue("OS", sValue.Render(nonempty(s.OS, "—")), col),
		labelValue("PLATFORM", sValue.Render(truncateVisible(nonempty(s.Platform, "—"), w-col-4)), col),
		labelValue("UPTIME", sValue.Render(formatUptime(s.Uptime)), col),
	}, "\n")
	return section("01", "KERNEL", body, w)
}

func (m model) renderNetwork(w int) string {
	s := m.snap
	col := 11
	sparkW := w - col - 4 - 14 // 14 for value slot
	if sparkW < 6 {
		sparkW = 6
	}
	tx := formatBytes(s.NetSent)
	rx := formatBytes(s.NetRecv)
	rate := formatRate(s.NetRate)
	pill := bytesRatePill(s.NetRate)

	spark := sparkline(s.NetHistory, 0, sparkW)

	line1 := labelValue("TX", sValue.Render(fmt.Sprintf("%-10s", tx)), col) + "  " + spark
	line2 := labelValue("RX", sValue.Render(fmt.Sprintf("%-10s", rx)), col)
	line3 := labelValue("RATE", sValue.Render(fmt.Sprintf("%-10s", rate))+"  "+pill, col)
	body := strings.Join([]string{line1, line2, line3, " "}, "\n")
	return section("02", "NETWORK", body, w)
}

func (m model) renderCPU(w int) string {
	s := m.snap
	col := 11
	gw := w - col - 4 - 14
	if gw < 6 {
		gw = 6
	}
	gauge := segmentedGauge(s.CPUUsage, gw)
	spark := sparkline(s.CPUHistory, 100, w-col-4)
	usageTxt := levelStyle(s.CPUUsage).Render(fmt.Sprintf("%5.1f%%", s.CPUUsage))

	line1 := labelValue("LOAD", usageTxt+"  "+statusPill(s.CPUUsage), col)
	line2 := labelValue("USAGE", gauge, col)
	line3 := labelValue("HIST", spark, col)
	body := strings.Join([]string{line1, line2, line3, " "}, "\n")
	return section("03", "CPU", body, w)
}

func (m model) renderMemory(w int) string {
	s := m.snap
	col := 11
	gw := w - col - 4 - 14
	if gw < 6 {
		gw = 6
	}
	pct := 0.0
	if s.MemoryTotal > 0 {
		pct = 100 * float64(s.MemoryUsed) / float64(s.MemoryTotal)
	}
	gauge := segmentedGauge(pct, gw)
	spark := sparkline(s.MemHistory, 100, w-col-4)

	used := fmt.Sprintf("%s / %s", formatBytes(s.MemoryUsed), formatBytes(s.MemoryTotal))
	free := formatBytes(s.MemoryFree)

	line1 := labelValue("USED", sValue.Render(used)+"  "+statusPill(pct), col)
	line2 := labelValue("FREE", sValue.Render(free), col)
	line3 := labelValue("USAGE", gauge, col)
	line4 := labelValue("HIST", spark, col)
	body := strings.Join([]string{line1, line2, line3, line4}, "\n")
	return section("04", "MEMORY", body, w)
}

func (m model) renderStorage(w int) string {
	s := m.snap
	if len(s.Disks) == 0 {
		return section("05", "STORAGE", sDim.Render("no disks reported"), w)
	}
	col := 11
	// compute widths
	mountW := 0
	for _, d := range s.Disks {
		if lipgloss.Width(d.Path) > mountW {
			mountW = lipgloss.Width(d.Path)
		}
	}
	if mountW < 6 {
		mountW = 6
	}
	sizeW := 22
	pctW := 7
	pillW := 10
	_ = col
	gw := w - 4 - mountW - 2 - sizeW - 2 - pctW - 2 - pillW - 2
	if gw < 8 {
		gw = 8
	}
	var rows []string
	for _, d := range s.Disks {
		mount := sPaper.Render(pad(d.Path, mountW))
		size := sValue.Render(pad(fmt.Sprintf("%s / %s", formatBytes(d.Used), formatBytes(d.Total)), sizeW))
		pct := levelStyle(d.UsedPercent).Render(pad(fmt.Sprintf("%5.1f%%", d.UsedPercent), pctW))
		gauge := segmentedGauge(d.UsedPercent, gw)
		pill := statusPill(d.UsedPercent)
		rows = append(rows, fmt.Sprintf("%s  %s  %s  %s  %s", mount, size, pct, gauge, pill))
	}
	return section("05", "STORAGE", strings.Join(rows, "\n"), w)
}

func (m model) renderTopProcs(w int) string {
	s := m.snap

	type col struct {
		title string
		marker string
		procs []stats.ProcessInfo
		fmt   func(stats.ProcessInfo) string
	}
	cols := []col{
		{"CPU", "◉", s.TopCPU, func(p stats.ProcessInfo) string { return fmt.Sprintf("%5.1f%%", p.CPU) }},
		{"MEMORY", "◉", s.TopMem, func(p stats.ProcessInfo) string { return formatBytes(p.Mem) }},
		{"DISK I/O", "◉", s.TopDisk, func(p stats.ProcessInfo) string { return formatRate(p.DiskRate) }},
		{"NETWORK", "◉", s.TopNet, func(p stats.ProcessInfo) string { return fmt.Sprintf("%d", p.NetConns) }},
	}

	inner := w - 4
	colsPerRow := 4
	switch {
	case inner < 48:
		colsPerRow = 1
	case inner < 88:
		colsPerRow = 2
	case inner < 120:
		colsPerRow = 3
	}
	gutter := 2
	colW := (inner - gutter*(colsPerRow-1)) / colsPerRow

	// For each column, produce a list of lines.
	colLines := make([][]string, len(cols))
	for i, c := range cols {
		colLines[i] = renderProcColumn(c.marker, c.title, c.procs, c.fmt, colW)
	}

	// Group into rows of colsPerRow columns, joining horizontally.
	var body []string
	for start := 0; start < len(cols); start += colsPerRow {
		end := min(start+colsPerRow, len(cols))
		group := colLines[start:end]
		// Normalize to same line count.
		maxLen := 0
		for _, g := range group {
			if len(g) > maxLen {
				maxLen = len(g)
			}
		}
		for i := range group {
			for len(group[i]) < maxLen {
				group[i] = append(group[i], strings.Repeat(" ", colW))
			}
		}
		for r := 0; r < maxLen; r++ {
			var parts []string
			for i := range group {
				parts = append(parts, group[i][r])
			}
			body = append(body, strings.Join(parts, strings.Repeat(" ", gutter)))
		}
		if end < len(cols) {
			body = append(body, strings.Repeat(" ", inner))
		}
	}

	return section("06", "TOP PROCESSES", strings.Join(body, "\n"), w)
}

func renderProcColumn(marker, title string, ps []stats.ProcessInfo, fmtVal func(stats.ProcessInfo) string, width int) []string {
	if width < 16 {
		width = 16
	}

	head := sAmber.Render(marker) + " " + sSectionTitle.Render(title)
	// Pad head to width
	headW := lipgloss.Width(head)
	if headW < width {
		head += strings.Repeat(" ", width-headW)
	}
	rule := sBorder.Render(strings.Repeat("─", width))

	lines := []string{head, rule}
	valW := 9
	rankW := 3
	nameW := width - rankW - 1 - valW - 1
	if nameW < 6 {
		nameW = 6
	}
	for i, p := range ps {
		rank := sDim.Render(fmt.Sprintf("%02d", i+1))
		name := truncateVisible(p.Name, nameW)
		nameStyled := sText.Render(pad(name, nameW))
		val := sValue.Render(padRight(fmtVal(p), valW))
		line := rank + " " + nameStyled + " " + val
		// Ensure total is width.
		lw := lipgloss.Width(line)
		if lw < width {
			line += strings.Repeat(" ", width-lw)
		}
		lines = append(lines, line)
	}
	for len(lines) < 8 {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return lines
}

// --- Footer -----------------------------------------------------------------

func (m model) renderFooter() string {
	keys := []string{
		sFooterKey.Render("[Q]") + sFooterText.Render("·QUIT"),
		sFooterKey.Render("[↑↓ J/K]") + sFooterText.Render("·SCROLL"),
		sFooterKey.Render("[G]") + sFooterText.Render("·TOP"),
		sFooterKey.Render("[P/␣]") + sFooterText.Render("·PAUSE"),
	}
	left := " " + strings.Join(keys, "   ")
	var right string
	if m.paused {
		right = sHot.Render(" [ PAUSED ] ")
	} else {
		right = sDim.Render(fmt.Sprintf(" sampling · %s ", time.Since(m.started).Truncate(time.Second)))
	}
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// --- Helpers ---------------------------------------------------------------

func joinH(gutter int, parts ...string) string {
	if gutter <= 0 {
		return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	}
	gutterStr := strings.Repeat(" ", gutter)
	spaced := make([]string, 0, 2*len(parts)-1)
	for i, p := range parts {
		if i > 0 {
			spaced = append(spaced, gutterStr)
		}
		spaced = append(spaced, p)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, spaced...)
}

func pad(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return strings.Repeat(" ", n-w) + s
}

func nonempty(s, fb string) string {
	if strings.TrimSpace(s) == "" {
		return fb
	}
	return s
}

func formatUptime(seconds uint64) string {
	d := time.Duration(seconds) * time.Second
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	mins := int(d / time.Minute)
	d -= time.Duration(mins) * time.Minute
	secs := int(d / time.Second)
	return fmt.Sprintf("T+ %03dd %02dh %02dm %02ds", days, hours, mins, secs)
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatRate(bps uint64) string {
	return formatBytes(bps) + "/s"
}

func bytesRatePill(bps uint64) string {
	// thresholds: <256KB/s NOM, <4MB/s OK, <32MB/s ELEV, else CRIT
	switch {
	case bps < 256*1024:
		return sGood.Render("[ NOM  ]")
	case bps < 4*1024*1024:
		return sAmber.Render("[  OK  ]")
	case bps < 32*1024*1024:
		return sHot.Render("[ ELEV ]")
	default:
		return sCrit.Render("[ CRIT ]")
	}
}

// --- Entry point ------------------------------------------------------------

// Start launches the UI. It starts a stats collector goroutine that runs until
// the program exits.
func Start(version string) error {
	monitor := stats.NewMonitor()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Run(ctx, time.Second)

	p := tea.NewProgram(newModel(monitor, version), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
