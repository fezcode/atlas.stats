package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Glyph sets used across widgets.
const (
	sparkSet    = " ▁▂▃▄▅▆▇█"
	gaugeOn     = '▰'
	gaugeOff    = '▱'
	sepGlyph    = "·"
	sectionOpen = "┤"
	sectionClose = "├"
)

// sparkline renders vals into `width` cells, scaled against maxHint (0 = auto).
func sparkline(vals []float64, maxHint float64, width int) string {
	if width <= 0 || len(vals) == 0 {
		return strings.Repeat(" ", max(0, width))
	}
	// Pick the tail of the series that fits.
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	maxV := maxHint
	if maxV <= 0 {
		for _, v := range vals {
			if v > maxV {
				maxV = v
			}
		}
	}
	if maxV <= 0 {
		maxV = 1
	}
	runes := []rune(sparkSet)
	var b strings.Builder
	// Left-pad with blanks so the spark is right-aligned (new data on the right).
	b.WriteString(strings.Repeat(" ", width-len(vals)))
	for _, v := range vals {
		if v < 0 {
			v = 0
		}
		if v > maxV {
			v = maxV
		}
		idx := int((v / maxV) * float64(len(runes)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(runes) {
			idx = len(runes) - 1
		}
		b.WriteRune(runes[idx])
	}
	return sAmber.Render(b.String())
}

// segmentedGauge returns a coloured segment bar: ▰▰▰▰▱▱▱▱
func segmentedGauge(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct / 100.0 * float64(width))
	if pct > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	fs := levelStyle(pct)
	return fs.Render(strings.Repeat(string(gaugeOn), filled)) +
		sBorder.Render(strings.Repeat(string(gaugeOff), width-filled))
}

// section renders a rounded box with the title inlined into the top border.
// width is the total outer width (including the two vertical borders).
func section(num, title, body string, width int) string {
	if width < 20 {
		width = 20
	}
	// Title piece: ┤ §NN  TITLE ├
	titleInner := sSectionKey.Render(fmt.Sprintf(" §%s  ", num)) +
		sSectionTitle.Render(title) +
		" "
	label := sBorder.Render(sectionOpen) + titleInner + sBorder.Render(sectionClose)
	labelW := lipgloss.Width(label)

	// Top: ╭── label ────────╮
	lead := 2
	fill := width - 2 - lead - labelW
	if fill < 1 {
		// Try shrinking the lead.
		lead = 1
		fill = width - 2 - lead - labelW
		if fill < 1 {
			fill = 1
		}
	}
	top := sBorder.Render("╭"+strings.Repeat("─", lead)) +
		label +
		sBorder.Render(strings.Repeat("─", fill)+"╮")

	// Body rows, padded to interior width.
	inner := width - 4 // 2 border cells + 1 space padding each side
	var rows []string
	for _, ln := range strings.Split(body, "\n") {
		w := lipgloss.Width(ln)
		if w < inner {
			ln = ln + strings.Repeat(" ", inner-w)
		} else if w > inner {
			// Truncate the raw string naively; callers should size correctly.
			ln = truncateVisible(ln, inner)
		}
		rows = append(rows,
			sBorder.Render("│")+" "+ln+" "+sBorder.Render("│"))
	}
	bottom := sBorder.Render("╰" + strings.Repeat("─", width-2) + "╯")

	return top + "\n" + strings.Join(rows, "\n") + "\n" + bottom
}

// truncateVisible cuts a (possibly styled) string to visible width n.
// Naive: lipgloss styling is block-wrapped, so we strip to plain when oversize.
func truncateVisible(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// labelValue produces "LABEL        value" with fixed label column, left-aligned.
func labelValue(label, value string, labelCol int) string {
	lbl := sLabel.Render(label)
	pad := labelCol - lipgloss.Width(lbl)
	if pad < 1 {
		pad = 1
	}
	return lbl + strings.Repeat(" ", pad) + value
}

// horiz joins parts with a thin separator pill.
func horiz(parts ...string) string {
	return strings.Join(parts, "  "+sBorder.Render(sepGlyph)+"  ")
}

