package main

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func readerModel(width, height int) model {
	m := screenModel(width, height)
	m.tasks[0].Body = strings.Repeat("A long task with enough content to scroll through the entire reader.\n", 40)
	m.refilter()
	m.moveTo(1) // The long task follows the critical-priority task.
	return m
}

func press(m model, key tea.KeyMsg) (model, tea.Cmd) {
	updated, cmd := m.Update(key)
	return updated.(model), cmd
}

func runeKey(key rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}}
}

func TestTaskPageScroll(t *testing.T) {
	for _, mode := range []string{"sidebar list focus", "sidebar detail focus", "fullscreen"} {
		t.Run(mode, func(t *testing.T) {
			m := readerModel(160, 14)
			if mode == "sidebar detail focus" {
				m.focus = focusDetail
			}
			if mode == "fullscreen" {
				m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
			}
			cursor := m.cursor
			m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlF})
			if m.detail.YOffset != m.detail.Height || m.cursor != cursor {
				t.Fatalf("ctrl+f: offset %d, height %d, cursor %d", m.detail.YOffset, m.detail.Height, m.cursor)
			}
			m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlB})
			if m.detail.YOffset != 0 || m.cursor != cursor {
				t.Fatalf("ctrl+b: offset %d, cursor %d", m.detail.YOffset, m.cursor)
			}
		})
	}
}

func TestReaderUsesFullScreenAndRestoresPreview(t *testing.T) {
	for _, focus := range []focusArea{focusList, focusDetail} {
		m := readerModel(160, 14)
		m.focus = focus
		m.detail.LineDown(3)
		previewOffset := m.detail.YOffset
		previewWidth, previewHeight := m.detail.Width, m.detail.Height
		task, _ := m.selected()

		m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
		if !m.reading || m.splitPanes() || m.detail.Width != m.width || m.detail.Height != m.height-1 {
			t.Fatal("enter should give the selected task the full screen")
		}
		if m.detail.YOffset != 0 {
			t.Fatal("opening a task should start at its title")
		}
		view := ansi.Strip(m.View())
		for _, hidden := range []string{m.proj.TaskDir, "sort:", "o:open", "╭", "Ship board view"} {
			if strings.Contains(view, hidden) {
				t.Errorf("reader still shows browser content %q", hidden)
			}
		}
		if !strings.Contains(view, task.Title) || !strings.Contains(view, "read only") || !strings.Contains(view, "esc back") {
			t.Fatalf("reader should show the task and explain the mode:\n%s", view)
		}
		for _, hidden := range []string{"e edit", "advance", "s sort", "/ filter"} {
			if strings.Contains(ansi.Strip(m.footerView()), hidden) {
				t.Errorf("reader advertises inactive shortcut %q", hidden)
			}
		}
		assertScreenFits(t, view, m.width, m.height)

		m, _ = press(m, runeKey('G'))
		if m.detail.YOffset == 0 {
			t.Fatal("reader navigation should scroll the task")
		}
		m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
		selected, _ := m.selected()
		if m.reading || m.focus != focus || selected.ID != task.ID || m.detail.YOffset != previewOffset {
			t.Fatal("esc should restore the browser focus, selected task and preview position")
		}
		if m.detail.Width != previewWidth || m.detail.Height != previewHeight {
			t.Fatal("esc should restore the preview dimensions")
		}
	}
}

func TestReaderIgnoresEditingAndBrowserShortcuts(t *testing.T) {
	m := readerModel(95, 14)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	tasks := append([]Task(nil), m.tasks...)
	filters, order, cursor := m.filters, m.order, m.cursor
	for _, key := range "e123456/ofFpPtThHwWnNyYocCsS" {
		var cmd tea.Cmd
		m, cmd = press(m, runeKey(key))
		if cmd != nil {
			t.Fatalf("read-only key %q issued a command", key)
		}
		if !m.reading || m.searching || !reflect.DeepEqual(m.tasks, tasks) || !reflect.DeepEqual(m.filters, filters) || m.order != order || m.cursor != cursor {
			t.Fatalf("key %q changed the task or browser state while reading", key)
		}
	}
	m, cmd := press(m, runeKey(' '))
	if cmd != nil || !reflect.DeepEqual(m.tasks, tasks) || m.detail.YOffset == 0 {
		t.Fatal("space should scroll, without issuing a status change")
	}

	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	for _, key := range "e 1" {
		_, cmd := press(m, runeKey(key))
		if cmd == nil {
			t.Errorf("browser action %q should work after leaving the reader", key)
		}
	}
}

func TestReaderSurvivesResizeAndReload(t *testing.T) {
	m := readerModel(160, 14)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	task, _ := m.selected()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = updated.(model)
	m, _ = press(m, runeKey('j'))
	offset := m.detail.YOffset

	tasks := append([]Task(nil), m.tasks...)
	tasks[0].Body += "External update at the end of the task."
	tasks = append([]Task{{ID: "000", Title: "New task", Priority: "critical"}}, tasks...)
	updated, _ = m.Update(tasksMsg{tasks: tasks})
	m = updated.(model)
	selected, _ := m.selected()
	if !m.reading || selected.ID != task.ID || m.detail.YOffset != offset || m.detail.Width != 40 {
		t.Fatal("resize and reload should keep the same task and reader position")
	}
	m, _ = press(m, runeKey('G'))
	if !strings.Contains(ansi.Strip(m.View()), "External update") {
		t.Fatal("reader should pick up external edits")
	}
	assertScreenFits(t, m.View(), 40, 10)

	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.reading || m.detail.Width != 38 {
		t.Fatal("leaving after a resize should restore the compact browser layout")
	}
	assertScreenFits(t, m.View(), 40, 10)
}

func TestReaderHelpOnlyShowsReaderKeys(t *testing.T) {
	m := readerModel(40, 10)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = press(m, runeKey('?'))
	if !m.showHelp || !m.reading {
		t.Fatal("reader should have its own help")
	}
	help := strings.Join(m.helpLines(), "\n")
	if strings.Contains(help, "advance status") || strings.Contains(help, "$EDITOR") || !strings.Contains(help, "page down") {
		t.Fatal("reader help should describe the read-only controls")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHelp || !m.reading {
		t.Fatal("closing help should return to the reader")
	}
	m, cmd := press(m, runeKey('e'))
	if cmd != nil || !m.reading {
		t.Fatal("closing help must not enable editing")
	}
}

func TestReaderRequiresASelectedTask(t *testing.T) {
	m := screenModel(95, 14)
	m.tasks = nil
	m.refilter()
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.reading || cmd != nil {
		t.Fatal("empty lists should not open a reader")
	}
}

func TestReaderReturnsToBrowserWhenTaskDisappears(t *testing.T) {
	m := readerModel(160, 14)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	task, _ := m.selected()
	var remaining []Task
	for _, candidate := range m.tasks {
		if candidate.ID != task.ID {
			remaining = append(remaining, candidate)
		}
	}
	updated, _ := m.Update(tasksMsg{tasks: remaining})
	m = updated.(model)
	if m.reading || !m.splitPanes() || !strings.Contains(m.notice, task.ID) {
		t.Fatal("a missing task should return to the browser with a notice")
	}
	assertScreenFits(t, m.View(), 160, 14)
}
