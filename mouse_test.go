package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func wheel(m model, x, y int, button tea.MouseButton) model {
	updated, _ := m.Update(tea.MouseMsg{X: x, Y: y, Button: button, Action: tea.MouseActionPress})
	return updated.(model)
}

func TestMouseScrollPanes(t *testing.T) {
	m := readerModel(160, 14)
	cursor := m.cursor
	m = wheel(m, m.listWidth(), m.headerHeight(), tea.MouseButtonWheelDown)
	if m.detail.YOffset == 0 || m.cursor != cursor || m.focus != focusList {
		t.Fatal("wheel over detail should scroll detail without changing selection or focus")
	}
	m = wheel(m, m.listWidth(), m.headerHeight(), tea.MouseButtonWheelUp)
	if m.detail.YOffset != 0 {
		t.Fatal("wheel up should scroll detail back to the top")
	}
	m.focus = focusDetail
	m = wheel(m, 0, m.headerHeight(), tea.MouseButtonWheelDown)
	if m.cursor <= cursor || m.focus != focusDetail {
		t.Fatal("wheel over list should move selection without changing focus")
	}
	cursor = m.cursor
	m = wheel(m, 0, 0, tea.MouseButtonWheelUp)
	if m.cursor != cursor {
		t.Fatal("wheel over header should not scroll the list")
	}
}

func TestMouseScrollViews(t *testing.T) {
	m := readerModel(80, 10)
	m.focus = focusDetail
	m = wheel(m, 0, m.headerHeight(), tea.MouseButtonWheelDown)
	if m.detail.YOffset == 0 {
		t.Fatal("wheel should scroll the compact detail view")
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = wheel(m, 0, 0, tea.MouseButtonWheelDown)
	if m.detail.YOffset == 0 {
		t.Fatal("wheel should scroll the reader")
	}
	m.showHelp = true
	m = wheel(m, 0, 0, tea.MouseButtonWheelDown)
	if m.helpOffset == 0 {
		t.Fatal("wheel should scroll help")
	}
	m.showHelp = false
	m.openStatusSelector()
	m = wheel(m, 0, 0, tea.MouseButtonWheelDown)
	if m.statusCursor != 1 {
		t.Fatal("wheel should navigate the status selector")
	}
}
