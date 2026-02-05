package ui

import (
	"fmt"
	"strings"
	"time"

	"atlas.stats/pkg/stats"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			MarginBottom(1)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ADADAD")).
			Width(15)

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	infoBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2).
			MarginRight(1)

	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FAFAFA")).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(lipgloss.Color("#555"))

	tableRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EEE"))
)

type tickMsg time.Time

type model struct {
	monitor  *stats.Monitor
	stats    stats.SystemStats
	cpuProg  progress.Model
	memProg  progress.Model
	diskProg progress.Model

	// State for bandwidth calc
	lastNetSent uint64
	lastNetRecv uint64
	netHistory  []uint64

	err          error
	width        int
	height       int
	scrollOffset int
}

func NewModel() model {
	return model{
		monitor:    stats.NewMonitor(),
		cpuProg:    progress.New(progress.WithDefaultGradient()),
		memProg:    progress.New(progress.WithDefaultGradient()),
		diskProg:   progress.New(progress.WithDefaultGradient()),
		netHistory: make([]uint64, 40),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tick(), m.updateStats())
}

func (m model) updateStats() tea.Cmd {
	return func() tea.Msg {
		s, err := m.monitor.GetStats()
		if err != nil {
			return err
		}
		return s
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "down", "j":
			m.scrollOffset++ // View will clamp this
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		barWidth := msg.Width/2 - 14
		if msg.Width < 80 {
			barWidth = msg.Width - 14
		}
		if barWidth < 10 {
			barWidth = 10
		}

		m.cpuProg.Width = barWidth
		m.memProg.Width = barWidth

	case tickMsg:
		return m, tea.Batch(tick(), m.updateStats())
	case stats.SystemStats:
		currTotal := msg.NetSent + msg.NetRecv
		prevTotal := m.lastNetSent + m.lastNetRecv
		bandwidth := uint64(0)

		if prevTotal > 0 && currTotal >= prevTotal {
			bandwidth = currTotal - prevTotal
		}

		m.netHistory = append(m.netHistory[1:], bandwidth)
		m.lastNetSent = msg.NetSent
		m.lastNetRecv = msg.NetRecv

		m.stats = msg
		return m, nil
	case error:
		m.err = msg
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	header := titleStyle.Render(" ATLAS STATS ")

	boxWidth := m.width/2 - 4
	isNarrow := m.width < 80
	if isNarrow {
		boxWidth = m.width - 4
	}

	// --- Row 1: System Info & Network ---
	sysInfo := infoBoxStyle.Width(boxWidth).Render(lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("%s %s", labelStyle.Render("Hostname:"), valueStyle.Render(m.stats.Hostname)),
		fmt.Sprintf("%s %s", labelStyle.Render("OS:"), valueStyle.Render(m.stats.OS)),
		fmt.Sprintf("%s %s", labelStyle.Render("Platform:"), valueStyle.Render(m.stats.Platform)),
		fmt.Sprintf("%s %s", labelStyle.Render("Uptime:"), valueStyle.Render(formatDuration(m.stats.Uptime))),
	))

	netRate := m.netHistory[len(m.netHistory)-1]
	netInfo := infoBoxStyle.Width(boxWidth).Render(lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Render("Network Activity"),
		fmt.Sprintf("%s %s", labelStyle.Render("Total Sent:"), valueStyle.Render(formatBytes(m.stats.NetSent))),
		fmt.Sprintf("%s %s", labelStyle.Render("Total Recv:"), valueStyle.Render(formatBytes(m.stats.NetRecv))),
		fmt.Sprintf("%s %s", labelStyle.Render("Current Rate:"), valueStyle.Render(fmt.Sprintf("%s/s", formatBytes(netRate)))),
	))

	var row1 string
	if isNarrow {
		row1 = lipgloss.JoinVertical(lipgloss.Left, sysInfo, netInfo)
	} else {
		row1 = lipgloss.JoinHorizontal(lipgloss.Top, sysInfo, netInfo)
	}

	// --- Row 2: CPU & Memory (Now 2nd as requested) ---
	cpuBar := m.cpuProg.ViewAs(m.stats.CPUUsage / 100)
	cpuInfo := infoBoxStyle.Width(boxWidth).Render(lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("%s %s", labelStyle.Render("CPU Usage:"), valueStyle.Render(fmt.Sprintf("%.2f%%", m.stats.CPUUsage))),
		cpuBar,
	))

	memUsage := 0.0
	if m.stats.MemoryTotal > 0 {
		memUsage = float64(m.stats.MemoryUsed) / float64(m.stats.MemoryTotal)
	}
	memBar := m.memProg.ViewAs(memUsage)
	memInfo := infoBoxStyle.Width(boxWidth).Render(lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("%s %s", labelStyle.Render("Memory:"), valueStyle.Render(fmt.Sprintf("%s / %s", formatBytes(m.stats.MemoryUsed), formatBytes(m.stats.MemoryTotal)))),
		memBar,
	))

	var row2 string
	if isNarrow {
		row2 = lipgloss.JoinVertical(lipgloss.Left, cpuInfo, memInfo)
	} else {
		row2 = lipgloss.JoinHorizontal(lipgloss.Top, cpuInfo, memInfo)
	}

	// --- Row 3: Disks (Now 3rd as requested) ---
	var diskBoxes []string
	diskCount := len(m.stats.Disks)
	if diskCount == 0 {
		diskBoxes = append(diskBoxes, infoBoxStyle.Width(m.width-4).Render("No disks found"))
	} else {
		diskBoxWidth := 44
		if isNarrow {
			diskBoxWidth = m.width - 4
		}

		m.diskProg.Width = diskBoxWidth - 10

		for _, d := range m.stats.Disks {
			bar := m.diskProg.ViewAs(d.UsedPercent / 100.0)
			view := infoBoxStyle.Width(diskBoxWidth).Render(lipgloss.JoinVertical(lipgloss.Left,
				fmt.Sprintf("%s %s", labelStyle.Render("Mount:"), valueStyle.Render(d.Path)),
				fmt.Sprintf("%s %s", labelStyle.Render("Usage:"), valueStyle.Render(fmt.Sprintf("%.1f%%", d.UsedPercent))),
				fmt.Sprintf("%s / %s", formatBytes(d.Used), formatBytes(d.Total)),
				bar,
			))
			diskBoxes = append(diskBoxes, view)
		}
	}
	row3 := wrapBoxes(diskBoxes, m.width)

	// --- Row 4: Top Processes ---
	topCPU := renderProcTable("Top CPU", m.stats.TopCPU, func(p stats.ProcessInfo) string {
		return fmt.Sprintf("%.1f%%", p.CPU)
	})

	topMem := renderProcTable("Top Mem", m.stats.TopMem, func(p stats.ProcessInfo) string {
		return formatBytes(p.Mem)
	})

	topDisk := renderProcTable("Top Disk I/O", m.stats.TopDisk, func(p stats.ProcessInfo) string {
		return formatBytes(p.DiskIO)
	})

	topNet := renderProcTable("Top Net Conns", m.stats.TopNet, func(p stats.ProcessInfo) string {
		return fmt.Sprintf("%d", p.NetConns)
	})

	row4 := wrapBoxes([]string{topCPU, topMem, topDisk, topNet}, m.width)

	fullContent := lipgloss.JoinVertical(lipgloss.Left,
		header,
		row1,
		row2,
		row3,
		row4,
		"\n Use ↑/↓ or j/k to navigate • Press 'q' to quit",
	)

	lines := strings.Split(fullContent, "\n")

	// Clamping scrollOffset
	maxOffset := len(lines) - m.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	visibleLines := lines[m.scrollOffset:]
	if len(visibleLines) > m.height {
		visibleLines = visibleLines[:m.height]
	}

	return strings.Join(visibleLines, "\n")
}

func wrapBoxes(boxes []string, maxWidth int) string {
	if len(boxes) == 0 {
		return ""
	}

	var finalRows []string
	var currentRow []string
	currentWidth := 0

	for _, box := range boxes {
		w := lipgloss.Width(box)
		if currentWidth+w > maxWidth && len(currentRow) > 0 {
			finalRows = append(finalRows, lipgloss.JoinHorizontal(lipgloss.Top, currentRow...))
			currentRow = nil
			currentWidth = 0
		}
		currentRow = append(currentRow, box)
		currentWidth += w
	}

	if len(currentRow) > 0 {
		finalRows = append(finalRows, lipgloss.JoinHorizontal(lipgloss.Top, currentRow...))
	}

	return lipgloss.JoinVertical(lipgloss.Left, finalRows...)
}

func renderProcTable(title string, procs []stats.ProcessInfo, valueFn func(stats.ProcessInfo) string) string {
	var rows []string
	rows = append(rows, tableHeaderStyle.Render(fmt.Sprintf("%-20s %10s", title, "Value")))

	for _, p := range procs {
		name := p.Name
		if len(name) > 18 {
			name = name[:15] + "..."
		}
		val := valueFn(p)
		rows = append(rows, tableRowStyle.Render(fmt.Sprintf("%-20s %10s", name, val)))
	}

	for len(rows) < 7 {
		rows = append(rows, " ")
	}

	return infoBoxStyle.Width(35).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func formatDuration(seconds uint64) string {
	d := time.Duration(seconds) * time.Second
	return d.String()
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

func Start() error {
	p := tea.NewProgram(NewModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
