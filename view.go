package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m model) View() string {
	if !m.ready {
		return "loading…"
	}
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.showHelp {
		return m.helpView()
	}

	body := m.listPane()
	if m.splitPanes() {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, m.detailPane())
	} else if m.reading || m.focus == focusDetail {
		body = m.detailPane()
	}

	var sections []string
	if m.headerHeight() > 0 {
		sections = append(sections, m.headerView())
	}
	sections = append(sections, body)
	if m.footerHeight() > 0 {
		sections = append(sections, m.footerView())
	}
	return strings.Join(sections, "\n")
}

// headerView is the two-line status block: where we are and how the list is
// currently sliced. The second line is the whole point of the TUI, so it is
// present even when nothing is filtered, unless height is very limited.
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
		prompt := truncate(m.search.View(), m.width)
		if m.headerHeight() == 1 {
			return prompt
		}
		return title + "\n" + prompt
	}
	if m.headerHeight() == 1 {
		return title
	}
	return title + "\n" + fitRow(m.filterLine(), m.sortLine(), m.width)
}

// filterLine spells out every active facet. Inactive ones are still listed, in
// muted text, so the keys that exist are discoverable without the help screen.
func (m model) filterLine() string {
	var active, inactive []string
	add := func(key, name, value string) {
		if value == anyValue {
			inactive = append(inactive, styleMuted.Render(key+":"+name))
			return
		}
		active = append(active, styleKey.Render(key)+":"+facetLabel(m.proj, name, value))
	}

	if raw := m.filters.Query.Raw; raw != "" {
		active = append(active, styleKey.Render("/")+styleSelected.Render(raw))
	}
	if m.filters.OpenOnly {
		active = append(active, styleKey.Render("o")+":"+styleSelected.Render("open"))
	} else {
		inactive = append(inactive, styleMuted.Render("o:open"))
	}
	add("f", "status", m.filters.Status)
	add("p", "prio", m.filters.Priority)
	add("t", "tag", m.filters.Tag)
	add("h", "phase", m.filters.Phase)
	add("w", "owner", m.filters.Owner)
	add("n", "group", m.filters.Group)
	add("y", "type", m.filters.Type)

	budget := m.width - lipgloss.Width(m.sortLine()) - 1
	for _, hints := range []string{
		strings.Join(inactive, " "),
		styleMuted.Render("o f p t h w n y") + " " + hint("?", "filters"),
		hint("?", "filters"),
		"",
	} {
		line := strings.Join(active, " ")
		if hints != "" {
			if line != "" {
				line += "  "
			}
			line += hints
		}
		if lipgloss.Width(line) <= budget || hints == "" {
			return line
		}
	}
	return ""
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
	inner := maxInt(0, width-m.paneFrameSize())
	height := m.listHeight()

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

	style := m.paneStyle(focusList)
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
		showPriority = width >= fixed+40+9
		showStatus   = width >= fixed+50+9+12
		showTags     = width >= fixed+60+9+12+18 && m.hasTags()
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

func (m model) hasTags() bool {
	for _, task := range m.tasks {
		if len(task.Tags) > 0 {
			return true
		}
	}
	return false
}

func (m model) paneStyle(focus focusArea) lipgloss.Style {
	if m.paneFrameSize() == 0 {
		return lipgloss.NewStyle()
	}
	style := styleBorder
	if m.focus == focus {
		style = styleBorderFocused
	}
	return style
}

func (m model) detailPane() string {
	width, height := m.detailSize()
	return m.paneStyle(focusDetail).Width(width).Height(height).Render(m.detail.View())
}

// renderDetail is the right-hand pane's content: the frontmatter as a field
// table, then the body.
func renderDetail(proj Project, task Task, width int) string {
	if width <= 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(ansi.Wrap(styleAccent.Render(task.ID)+"  "+styleTitle.Render(task.Title), width, "") + "\n")
	b.WriteString(styleMuted.Render(strings.Repeat("─", width)) + "\n")

	field := func(name, value string) {
		if value == "" {
			return
		}
		if width < 24 {
			b.WriteString(ansi.Wrap(styleMuted.Render(name), width, "") + "\n")
			b.WriteString(wrapWithPrefix("  ", value, width) + "\n")
		} else {
			b.WriteString(wrapWithPrefix(styleMuted.Render(pad(name, 10)), value, width) + "\n")
		}
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
	if width <= 0 {
		return ""
	}

	var out []string
	inCode := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			inCode = !inCode
			out = append(out, styleMuted.Render(ansi.Hardwrap(line, width, true)))
		case inCode:
			out = append(out, styleMuted.Render(ansi.Hardwrap(line, width, true)))
		case strings.HasPrefix(trimmed, "#"):
			out = append(out, styleTitle.Render(ansi.Wrap(strings.TrimSpace(strings.TrimLeft(trimmed, "#")), width, "")))
		case strings.HasPrefix(trimmed, "- [x]"), strings.HasPrefix(trimmed, "- [X]"):
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			out = append(out, wrapWithPrefix(indent+styleNotice.Render("✓ "), styleMuted.Render(strings.TrimSpace(trimmed[5:])), width))
		case strings.HasPrefix(trimmed, "- [ ]"):
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			out = append(out, wrapWithPrefix(indent+styleAccent.Render("☐ "), strings.TrimSpace(trimmed[5:]), width))
		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "+ "):
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			out = append(out, wrapWithPrefix(indent+trimmed[:2], trimmed[2:], width))
		default:
			out = append(out, ansi.Wrap(line, width, ""))
		}
	}
	return strings.Join(out, "\n")
}

// Keep continuation lines under the value, rather than under its label or
// checkbox. Very narrow panes drop indentation to leave room for text.
func wrapWithPrefix(prefix, text string, width int) string {
	if width <= 0 {
		return ""
	}
	indent := lipgloss.Width(prefix)
	if indent >= width {
		return ansi.Wrap(prefix+text, width, "")
	}
	lines := strings.Split(ansi.Wrap(text, width-indent, ""), "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
		} else {
			lines[i] = strings.Repeat(" ", indent) + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) footerView() string {
	if m.reading {
		return m.readerFooterView()
	}
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

	navigation := hint("tab", "panes")
	if !m.splitPanes() {
		navigation = hint("tab", "details")
		if m.focus == focusDetail {
			navigation = hint("esc", "back")
		}
	}
	primary := []string{hint("/", "filter"), hint("enter", "read")}
	extraNavigation := navigation
	if m.focus == focusDetail {
		primary = []string{navigation, hint("j/k", "scroll")}
		extraNavigation = hint("enter", "read")
	}
	tail := []string{hint("?", "help"), hint("q", "quit")}
	join := func(parts []string) string { return strings.Join(parts, styleMuted.Render(" · ")) }
	withTail := func(parts []string) string {
		return join(append(append([]string{}, parts...), tail...))
	}
	if lipgloss.Width(withTail(primary)) > m.width {
		primary = []string{primary[1]}
		if m.focus == focusDetail {
			primary = []string{navigation}
		}
	}
	if lipgloss.Width(withTail(primary)) > m.width {
		primary = nil
	}
	for _, extra := range []string{
		extraNavigation,
		hint("s", "sort"), hint("e", "edit"), hint("o", "open"),
		hint("space", "advance"), hint("c", "clear"),
	} {
		candidate := append(append([]string{}, primary...), extra)
		if lipgloss.Width(withTail(candidate)) <= m.width {
			primary = candidate
		}
	}
	return truncate(withTail(primary), m.width)
}

func (m model) readerFooterView() string {
	line := styleMuted.Render("read only") + styleMuted.Render(" · ") + hint("esc", "back")
	if lipgloss.Width(line) > m.width {
		return fitRow(styleMuted.Render("read only"), hint("esc", "back"), m.width)
	}
	for _, extra := range []string{hint("j/k", "scroll"), hint("?", "help"), hint("q", "quit")} {
		candidate := line + styleMuted.Render(" · ") + extra
		if lipgloss.Width(candidate) <= m.width {
			line = candidate
		}
	}
	return line
}

func hint(key, label string) string {
	return styleKey.Render(key) + styleMuted.Render(" "+label)
}

// helpRows is the full keymap, and the only place it is written down for the
// reader. The dispatch lives in handleKey.
type helpRow struct{ keys, label string }

var helpRows = []helpRow{
	{"j / k, ↑ / ↓", "move"},
	{"ctrl+d / ctrl+u", "half page"},
	{"ctrl+f / ctrl+b", "scroll task one page"},
	{"g / G", "first / last"},
	{"tab", "switch pane (scrolls the detail when focused)"},
	{"enter", "read the selected task full screen; esc returns"},
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

var readerHelpRows = []helpRow{
	{"j / k, ↑ / ↓", "scroll the task"},
	{"ctrl+d / ctrl+u", "half page"},
	{"ctrl+f / ctrl+b", "full page"},
	{"pgdown / pgup", "half page"},
	{"space", "page down"},
	{"g / G", "top / bottom"},
	{"r", "reload now (the task also reloads every 2s)"},
	{"esc, enter, tab", "return to the browser"},
	{"?", "close this help"},
	{"q", "quit"},
}

// helpView fills the screen with the keymap. It scrolls rather than clips: the
// syntax reference below the keys is worth more than a terminal-height budget,
// and j/k already mean "move" everywhere else.
func (m model) helpView() string {
	lines := m.helpLines()

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
	return strings.Join(append(lines[offset:offset+body:offset+body], styleMuted.Render(truncate(hint, m.width))), "\n")
}

// helpLength is how many lines the help page runs to, for clamping its scroll.
func (m model) helpLength() int { return len(m.helpLines()) }

func (m model) helpLines() []string {
	if m.reading {
		return helpLinesFor(m.width, readerHelpRows, []string{
			"Read only: editing and status changes are disabled while reading a task.",
		})
	}
	return helpLines(m.width)
}

func helpLines(width int) []string {
	return helpLinesFor(width, helpRows, []string{
		"Uppercase cycles a filter backwards. Facets and the query are ANDed.",
		"Status changes are written by `taskmd set`; the list reloads every 2s.",
	})
}

func helpLinesFor(width int, rows []helpRow, notes []string) []string {
	var lines []string
	add := func(line string) {
		lines = append(lines, strings.Split(line, "\n")...)
	}
	add(ansi.Wrap(styleTitle.Render("taskmd-ui")+styleMuted.Render("  keys — ? or esc closes"), width, ""))
	add("")
	for _, row := range rows {
		switch {
		case row.keys == "" && row.label == "":
			add("")
		case row.keys == "":
			// A continuation of the row above: the query syntax under `/`.
			add(wrapWithPrefix("  ", styleMuted.Render(strings.TrimSpace(row.label)), width))
		case width < 64:
			add(wrapWithPrefix("  ", styleKey.Render(row.keys)+"  "+row.label, width))
		default:
			add(wrapWithPrefix("  "+styleKey.Render(pad(row.keys, 18))+" ", row.label, width))
		}
	}
	add("")
	for _, note := range notes {
		add(wrapWithPrefix("  ", styleMuted.Render(note), width))
	}
	return lines
}

// fitRow puts left and right on one line, padding between them and dropping
// text from the left so counts and the sort order stay visible.
func fitRow(left, right string, width int) string {
	if lipgloss.Width(right) >= width {
		return truncate(right, width)
	}
	left = truncate(left, width-lipgloss.Width(right)-1)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	return left + strings.Repeat(" ", gap) + right
}
