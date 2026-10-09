package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func screenModel(width, height int) model {
	m := newModel(Project{TaskDir: "/home/aldur/heartbit.technology/tasks"})
	m.tasks = sample()
	m.tasks[0].Title = "Verify Cloudflare data handling and international transfer arrangements"
	m.tasks[0].Owner = "a-very-long-owner-name-without-spaces"
	m.tasks[0].Rel = "compliance/001-verify-cloudflare-data-handling.md"
	m.tasks[0].Body = "## Tasks\n\n- [ ] Inspect Pages Web Analytics, Zaraz, Turnstile, Workers routes and logging/export products.\n- [x] Record documented retention and account agreements."
	m.refilter()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(model)
}

func assertScreenFits(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("view has %d rows in a %d-row terminal:\n%s", len(lines), height, ansi.Strip(view))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("row %d has width %d in a %d-column terminal: %q", i, got, width, ansi.Strip(line))
		}
	}
}

func TestViewsFitSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {2, 3}, {3, 4}, {20, 6}, {40, 10}, {80, 24}, {95, 28}, {119, 20}, {120, 24}, {160, 40}} {
		for _, state := range []string{"list", "detail", "reader", "reader help", "search", "help", "empty", "error"} {
			t.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], state), func(t *testing.T) {
				m := screenModel(size[0], size[1])
				m.moveTo(1)
				switch state {
				case "detail":
					m.focus = focusDetail
				case "reader", "reader help":
					updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
					m = updated.(model)
					if state == "reader help" {
						m.showHelp = true
						m.scrollHelp(m.helpLength() / 2)
					}
				case "search":
					m.searching = true
					m.search.SetValue("priority>=high status!=completed owner:a-very-long-owner-name")
					m.search.CursorEnd()
				case "help":
					m.showHelp = true
					m.scrollHelp(m.helpLength() / 2)
				case "empty":
					m.tasks = nil
					m.refilter()
				case "error":
					m.loadErr = fmt.Errorf("cannot read tasks in a directory with a very long name")
				}
				assertScreenFits(t, m.View(), size[0], size[1])
			})
		}
	}
}

func TestCompactLayoutAndNavigation(t *testing.T) {
	m := screenModel(95, 28)
	if m.splitPanes() || m.listWidth() != 95 || m.detail.Width != 93 {
		t.Fatal("compact terminals should give each view the full width")
	}
	row := ansi.Strip(m.taskRow(m.tasks[0], true, m.listWidth()-m.paneFrameSize()))
	if !strings.Contains(row, "Verify Cloudflare data handling and international") {
		t.Fatalf("optional columns should leave room for the title: %q", row)
	}
	footer := ansi.Strip(m.footerView())
	for _, hint := range []string{"tab details", "? help", "q quit"} {
		if !strings.Contains(footer, hint) {
			t.Errorf("footer %q is missing %q", footer, hint)
		}
	}

	m = screenModel(40, 8)
	m.moveTo(len(m.visible) - 1)
	task, _ := m.selected()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.focus != focusDetail || !strings.Contains(ansi.Strip(m.footerView()), "esc back") {
		t.Fatal("tab should open the details with a way back to the list")
	}
	updated, _ = m.Update(tasksMsg{tasks: m.tasks})
	m = updated.(model)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 9})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	selected, _ := m.selected()
	if selected.ID != task.ID || m.cursor < m.offset || m.cursor >= m.offset+m.listHeight() {
		t.Fatal("returning after a reload and resize should keep the selected task in view")
	}

	updated, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	m = updated.(model)
	if !m.splitPanes() || !strings.Contains(m.View(), "╮╭") {
		t.Fatal("wide terminals should still show both panes")
	}
}

func TestCompactHeaderKeepsCountSortAndActiveFilters(t *testing.T) {
	m := screenModel(50, 10)
	m.filters.Tag = "cli"
	m.refilter()
	header := ansi.Strip(m.headerView())
	for _, text := range []string{"2/5 tasks", "sort:priority↑", "t:cli"} {
		if !strings.Contains(header, text) {
			t.Errorf("header %q is missing %q", header, text)
		}
	}
}

func TestDetailWrappingPreservesText(t *testing.T) {
	m := screenModel(95, 28)
	task := m.tasks[0]
	for _, width := range []int{18, 30, 52, 93} {
		view := renderDetail(m.proj, task, width)
		assertScreenFits(t, view, width, 200)
		text := strings.Join(strings.Fields(ansi.Strip(view)), "")
		for _, value := range []string{task.Title, task.Owner, task.Rel, "Inspect Pages Web Analytics, Zaraz, Turnstile, Workers routes and logging/export products.", "Record documented retention and account agreements."} {
			if !strings.Contains(text, strings.Join(strings.Fields(value), "")) {
				t.Errorf("width %d clipped %q:\n%s", width, value, ansi.Strip(view))
			}
		}
	}
}

func TestCheckboxContinuationIndent(t *testing.T) {
	for _, checkbox := range []string{"- [ ]", "- [x]"} {
		view := renderBody("  "+checkbox+" Inspect Pages Web Analytics and record documented retention.", 24)
		assertScreenFits(t, view, 24, 20)
		lines := strings.Split(ansi.Strip(view), "\n")
		if len(lines) < 2 {
			t.Fatal("long checkbox text should wrap")
		}
		for _, line := range lines[1:] {
			if !strings.HasPrefix(line, "    ") {
				t.Errorf("continuation should align with checkbox text: %q", line)
			}
		}
	}
}

func TestNarrowHelpPreservesDescriptions(t *testing.T) {
	lines := helpLines(30)
	view := strings.Join(lines, "\n")
	assertScreenFits(t, view, 30, 500)
	text := strings.Join(strings.Fields(ansi.Strip(view)), "")
	for _, row := range helpRows {
		if !strings.Contains(text, strings.Join(strings.Fields(row.label), "")) {
			t.Errorf("help clipped %q", row.label)
		}
	}

	m := screenModel(30, 10)
	m.showHelp = true
	m.scrollHelp(m.helpLength())
	if m.helpOffset != len(lines)-(m.height-1) {
		t.Fatal("help should scroll to the bottom of the wrapped text")
	}
	assertScreenFits(t, m.View(), 30, 10)
}

func TestHalfPageOnOneRowTerminal(t *testing.T) {
	m := screenModel(40, 6)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if updated.(model).cursor != 1 {
		t.Fatal("half-page navigation should move at least one task")
	}
}

func TestTruncatePreservesANSIAndGraphemes(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
		want  string
	}{
		{"\x1b[1;36mCloudflare\x1b[0m", 5, "Clou…"},
		{"\x1b[31m界界界\x1b[0m", 4, "界…"},
		{"e\u0301clair", 2, "e\u0301…"},
		{"hello", 0, ""},
	} {
		got := truncate(tc.text, tc.width)
		if ansi.Strip(got) != tc.want || lipgloss.Width(got) > tc.width {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.text, tc.width, got, tc.want)
		}
		if strings.Contains(tc.text, "\x1b") && !strings.HasSuffix(got, "\x1b[0m") {
			t.Errorf("truncation lost the ANSI reset: %q", got)
		}
	}
}
