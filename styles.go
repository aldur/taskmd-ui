package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The palette is deliberately all ANSI 0-15: those are the colours the
// terminal theme defines, so the TUI inherits whatever scheme the user runs
// rather than fighting it with hardcoded hex.
var (
	colorMuted     = lipgloss.Color("8")
	colorAccent    = lipgloss.Color("4")
	colorHighlight = lipgloss.Color("6")

	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleAccent   = lipgloss.NewStyle().Foreground(colorAccent)
	styleKey      = lipgloss.NewStyle().Foreground(colorHighlight).Bold(true)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(colorHighlight)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleNotice   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	// The focused pane gets the brighter border, the way lazygit marks which
	// panel keys are going to.
	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted)
	styleBorderFocused = styleBorder.BorderForeground(colorHighlight)
)

// statusStyles pairs each status with a glyph and a colour: the glyph carries
// the state where colour cannot (monochrome terminals, colourblind readers),
// the colour makes a long list scannable.
var statusStyles = map[string]struct {
	Glyph string
	Color lipgloss.Color
}{
	"pending":     {"○", lipgloss.Color("7")},
	"in-progress": {"◐", lipgloss.Color("3")},
	"in-review":   {"◑", lipgloss.Color("6")},
	"blocked":     {"⊘", lipgloss.Color("1")},
	"completed":   {"●", lipgloss.Color("2")},
	"cancelled":   {"⊗", lipgloss.Color("8")},
}

func statusGlyph(status string) string {
	if s, ok := statusStyles[status]; ok {
		return lipgloss.NewStyle().Foreground(s.Color).Render(s.Glyph)
	}
	return styleMuted.Render("·")
}

func statusText(status string) string {
	if s, ok := statusStyles[status]; ok {
		return lipgloss.NewStyle().Foreground(s.Color).Render(status)
	}
	return styleMuted.Render(orDash(status))
}

var priorityColors = map[string]lipgloss.Color{
	"critical": lipgloss.Color("1"),
	"high":     lipgloss.Color("3"),
	"medium":   lipgloss.Color("4"),
	"low":      lipgloss.Color("8"),
}

func priorityText(priority string) string {
	if c, ok := priorityColors[priority]; ok {
		return lipgloss.NewStyle().Foreground(c).Render(priority)
	}
	return styleMuted.Render(orDash(priority))
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// truncate cuts a string to a display width, using a single-character ellipsis
// so the budget is exact.
func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(text, width, "…")
}

// pad right-fills to an exact display width, so columns line up even when a
// title carries wide runes.
func pad(text string, width int) string {
	text = truncate(text, width)
	if gap := width - lipgloss.Width(text); gap > 0 {
		return text + strings.Repeat(" ", gap)
	}
	return text
}
