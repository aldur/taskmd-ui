package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	headerHeight = 2
	footerHeight = 1
)

func (m model) View() string {
	if !m.ready {
		return "loading…"
	}
	if m.showHelp {
		return m.helpView()
	}

	body := m.listPane()
	if m.splitPanes() {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, m.detailPane())
	} else if m.focus == focusDetail {
		body = m.detailPane()
	}

	return strings.Join([]string{m.headerView(), body, m.footerView()}, "\n")
}

// headerView is the two-line status block: where we are and how the list is
// currently sliced. The second line is the whole point of the TUI, so it is
// always present even when nothing is filtered.
func (m model) headerView() string {
	left := styleTitle.Render("taskmd") + " " + styleMuted.Render(m.proj.TaskDir)
	count := fmt.Sprintf("%d/%s", len(m.visible), plural(len(m.tasks), "task"))
	if m.loadErr != nil {
		count = styleError.Render("scan failed")
	}
	title := fitRow(left, styleAccent.Render(count), m.width)

	// While typing, the line belongs to the prompt — a long expression needs
	// the room, and the footer carries any complaint about it.
	if m.searching {
		return title + "\n" + m.search.View()
	}
	return title + "\n" + fitRow(m.filterLine(), m.sortLine(), m.width)
}

// filterLine spells out every active facet. Inactive ones are still listed, in
// muted text, so the keys that exist are discoverable without the help screen.
func (m model) filterLine() string {
	var parts []string
	add := func(key, name, value string) {
		if value == anyValue {
			parts = append(parts, styleMuted.Render(key+":"+name))
			return
		}
		parts = append(parts, styleKey.Render(key)+":"+facetLabel(m.proj, name, value))
	}

	if m.filters.OpenOnly {
		parts = append(parts, styleKey.Render("o")+":"+styleSelected.Render("open"))
	} else {
		parts = append(parts, styleMuted.Render("o:open"))
	}
	add("f", "status", m.filters.Status)
	add("p", "prio", m.filters.Priority)
	add("t", "tag", m.filters.Tag)
	add("h", "phase", m.filters.Phase)
	add("w", "owner", m.filters.Owner)
	add("n", "group", m.filters.Group)
	add("y", "type", m.filters.Type)

	line := strings.Join(parts, " ")
	if raw := m.filters.Query.Raw; raw != "" {
		line = styleKey.Render("/") + styleSelected.Render(raw) + "  " + line
	}
	return line
}

// facetLabel renders a facet's value, spelling the `none` sentinel out and
// giving a phase its configured display name.
func facetLabel(proj Project, name, value string) string {
	if value == noneValue {
		return styleSelected.Render("(no " + name + ")")
	}
	if name == "phase" {
		value = proj.phaseLabel(value)
	}
	return styleSelected.Render(value)
}

func (m model) sortLine() string {
	arrow := "↑"
	if m.order.Reverse {
		arrow = "↓"
	}
	return styleMuted.Render("sort:") + styleSelected.Render(m.order.Field.String()+arrow)
}

func (m model) listPane() string {
	width := m.listWidth()
	inner := maxInt(0, width-2)
	height := maxInt(0, m.bodyHeight()-2)

	var rows []string
	switch {
	case m.loadErr != nil:
		rows = append(rows, styleError.Render(truncate(m.loadErr.Error(), inner)))
	case len(m.visible) == 0 && len(m.tasks) > 0:
		rows = append(rows, styleMuted.Render(truncate("no task matches these filters — c clears them", inner)))
	case len(m.visible) == 0:
		rows = append(rows, styleMuted.Render(truncate("no tasks in "+m.proj.TaskDir, inner)))
	}

	end := minInt(m.offset+height, len(m.visible))
	for i := m.offset; i < end; i++ {
		rows = append(rows, m.taskRow(m.visible[i], i == m.cursor, inner))
	}
	for len(rows) < height {
		rows = append(rows, "")
	}

	style := styleBorder
	if m.focus == focusList {
		style = styleBorderFocused
	}
	return style.Width(inner).Height(height).Render(strings.Join(rows[:height], "\n"))
}

// taskRow lays out one line of the list, dropping columns from the right as
// the pane narrows so the title — the part you actually scan — keeps its room.
func (m model) taskRow(task Task, selected bool, width int) string {
	idWidth := minInt(m.idWidth(), 12)

	marker := "  "
	if selected {
		marker = styleSelected.Render("▸ ")
	}

	fixed := 2 + 2 + idWidth + 2 // marker, glyph, id, gap
	var (
		showPriority = width > fixed+30
		showStatus   = width > fixed+45
		showTags     = width > fixed+70
	)

	trailing := 0
	if showPriority {
		trailing += 9
	}
	if showStatus {
		trailing += 12
	}
	if showTags {
		trailing += 18
	}

	titleWidth := maxInt(6, width-fixed-trailing)
	// Truncate a rune short of the column so a clipped title keeps a gap
	// before the next one, rather than running "…" straight into "low".
	title := pad(truncate(task.Title, titleWidth-1), titleWidth)
	if selected {
		title = styleSelected.Render(title)
	} else if !task.IsOpen() {
		// Done and cancelled work recedes rather than disappearing.
		title = styleMuted.Render(title)
	}

	row := marker + statusGlyph(task.Status) + " " + styleAccent.Render(pad(task.ID, idWidth)) + "  " + title
	if showPriority {
		row += pad(priorityText(task.Priority), 9)
	}
	if showStatus {
		row += pad(statusText(task.Status), 12)
	}
	if showTags {
		row += styleMuted.Render(pad(strings.Join(task.Tags, ","), 18))
	}
	return truncate(row, width)
}

// idWidth sizes the id column to the widest id actually loaded, so `001` does
// not reserve room for `cli-049` in a project that has none.
func (m model) idWidth() int {
	width := 2
	for _, t := range m.tasks {
		width = maxInt(width, lipgloss.Width(t.ID))
	}
	return width
}

func (m model) detailPane() string {
	style := styleBorder
	if m.focus == focusDetail {
		style = styleBorderFocused
	}
	width, height := m.detailSize()
	return style.Width(width).Height(height).Render(m.detail.View())
}

// renderDetail is the right-hand pane's content: the frontmatter as a field
// table, then the body.
func renderDetail(proj Project, task Task, width int) string {
	if width <= 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(styleAccent.Render(task.ID) + "  " + styleTitle.Render(task.Title) + "\n")
	b.WriteString(styleMuted.Render(strings.Repeat("─", width)) + "\n")

	field := func(name, value string) {
		if value == "" {
			return
		}
		b.WriteString(styleMuted.Render(pad(name, 10)) + value + "\n")
	}
	field("status", statusText(task.Status))
	field("priority", priorityText(task.Priority))
	field("effort", task.Effort)
	field("type", task.Type)
	field("owner", task.Owner)
	if task.Phase != "" {
		field("phase", proj.phaseLabel(task.Phase))
	}
	field("group", task.Group)
	field("parent", task.Parent)
	field("tags", strings.Join(task.Tags, ", "))
	field("depends", strings.Join(task.Deps, ", "))
	field("touches", strings.Join(task.Touches, ", "))
	if !task.Created.IsZero() {
		field("created", task.Created.Format("2006-01-02"))
	}
	field("file", task.Rel)

	if task.Body != "" {
		b.WriteString(styleMuted.Render(strings.Repeat("─", width)) + "\n")
		b.WriteString(renderBody(task.Body, width))
	}
	return b.String()
}

// renderBody is a deliberately small markdown pass: headings, list items and
// checkboxes are what task bodies are made of, and a full renderer would cost
// a dependency the size of the rest of this program.
func renderBody(body string, width int) string {
	wrap := lipgloss.NewStyle().Width(width)

	var out []string
	inCode := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			inCode = !inCode
			out = append(out, styleMuted.Render(line))
		case inCode:
			out = append(out, styleMuted.Render(line))
		case strings.HasPrefix(trimmed, "#"):
			out = append(out, styleTitle.Render(wrap.Render(strings.TrimSpace(strings.TrimLeft(trimmed, "#")))))
		case strings.HasPrefix(trimmed, "- [x]"), strings.HasPrefix(trimmed, "- [X]"):
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			out = append(out, indent+styleNotice.Render("✓ ")+styleMuted.Render(strings.TrimSpace(trimmed[5:])))
		case strings.HasPrefix(trimmed, "- [ ]"):
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			out = append(out, indent+styleAccent.Render("☐ ")+strings.TrimSpace(trimmed[5:]))
		default:
			out = append(out, wrap.Render(line))
		}
	}
	return strings.Join(out, "\n")
}

func (m model) footerView() string {
	if notice, isErr := m.noticeText(); notice != "" {
		style := styleNotice
		if isErr {
			style = styleError
		}
		return style.Render(truncate(notice, m.width))
	}
	// A query that does not parse says so here, and keeps filtering on the
	// terms that did: a complaint, not a wall.
	if err := m.filters.Query.Err; err != "" {
		return styleError.Render(truncate(err, m.width))
	}
	if m.searching {
		return styleMuted.Render(truncate("status!=completed · priority>=high · tag:a11y,ux · owner:none · created<2026-06-01 · ? for the syntax", m.width))
	}

	hints := []string{
		hint("/", "filter"),
		hint("o", "open"),
		hint("s", "sort"),
		hint("space", "advance"),
		hint("e", "edit"),
		hint("c", "clear"),
		hint("?", "help"),
		hint("q", "quit"),
	}
	return truncate(strings.Join(hints, styleMuted.Render(" · ")), m.width)
}

func hint(key, label string) string {
	return styleKey.Render(key) + styleMuted.Render(" "+label)
}

// helpRows is the full keymap, and the only place it is written down for the
// reader. The dispatch lives in handleKey.
var helpRows = []struct{ keys, label string }{
	{"j / k, ↑ / ↓", "move"},
	{"ctrl+d / ctrl+u", "half page"},
	{"g / G", "first / last"},
	{"tab, enter", "switch pane (scrolls the detail when focused)"},
	{"", ""},
	{"/", "filter as you type; enter keeps it, esc reverts"},
	{"", "  parser              free text over id, title, tags and body"},
	{"", "  -parser             …and not that"},
	{"", "  \"record format\"     quoted, so the space is part of it"},
	{"", "  status:blocked      field is this value"},
	{"", "  status!=completed   field is not (same as -status:completed)"},
	{"", "  status:open         open, active, done or resolved as a group"},
	{"", "  tag:a11y,ux         either value"},
	{"", "  priority>=high      at least this urgent (effort orders too)"},
	{"", "  created<2026-06-01  dates compare as dates"},
	{"", "  owner:none          field is unset (owner:any for the opposite)"},
	{"", "  fields: status priority effort type owner phase group tag dep"},
	{"", "          touches parent id title file body created"},
	{"", ""},
	{"o", "toggle open-only (pending, in-progress, in-review, blocked)"},
	{"f / F", "cycle status filter"},
	{"p / P", "cycle priority filter"},
	{"t / T", "cycle tag filter"},
	{"h / H", "cycle phase filter"},
	{"w / W", "cycle owner filter"},
	{"n / N", "cycle group filter"},
	{"y / Y", "cycle type filter"},
	{"c", "clear every filter"},
	{"", ""},
	{"s", "cycle sort field (priority, status, id, created, modified, effort, title)"},
	{"S", "reverse the sort"},
	{"", ""},
	{"space", "advance status along the workflow"},
	{"1 … 6", "set status: pending, in-progress, in-review, blocked, completed, cancelled"},
	{"e", "open the task in $EDITOR"},
	{"r", "reload now (the list also reloads every 2s)"},
	{"", ""},
	{"?", "close this help"},
	{"q", "quit"},
}

// helpView fills the screen with the keymap. It scrolls rather than clips: the
// syntax reference below the keys is worth more than a terminal-height budget,
// and j/k already mean "move" everywhere else.
func (m model) helpView() string {
	lines := helpLines()
	for i, line := range lines {
		lines[i] = truncate(line, m.width)
	}

	height := maxInt(1, m.height)
	if len(lines) <= height {
		return strings.Join(lines, "\n")
	}

	// One row goes to the scroll hint, so it is never a surprise that there
	// is more below.
	body := height - 1
	offset := minInt(maxInt(m.helpOffset, 0), len(lines)-body)
	remaining := len(lines) - body - offset
	hint := fmt.Sprintf("  … %d more line(s) — j/k scrolls, ? or esc closes", remaining)
	if remaining == 0 {
		hint = "  ↑ j/k scrolls, ? or esc closes"
	}
	return strings.Join(append(lines[offset:offset+body:offset+body], styleMuted.Render(hint)), "\n")
}

// helpLength is how many lines the help page runs to, for clamping its scroll.
func helpLength() int { return len(helpLines()) }

func helpLines() []string {
	lines := []string{
		styleTitle.Render("taskmd-ui") + styleMuted.Render("  keys — ? or esc closes"),
		"",
	}
	for _, row := range helpRows {
		switch {
		case row.keys == "" && row.label == "":
			lines = append(lines, "")
		case row.keys == "":
			// A continuation of the row above: the query syntax under `/`.
			lines = append(lines, "  "+styleMuted.Render(row.label))
		default:
			lines = append(lines, "  "+styleKey.Render(pad(row.keys, 18))+" "+row.label)
		}
	}
	return append(lines, "",
		styleMuted.Render("  Uppercase cycles a filter backwards. Facets and the query are ANDed."),
		styleMuted.Render("  Status changes are written by `taskmd set`; the list reloads every 2s."))
}

// fitRow puts left and right on one line, padding between them and dropping
// the right side when the terminal is too narrow to hold both.
func fitRow(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return truncate(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}
