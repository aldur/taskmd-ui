package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestStatusSelector(t *testing.T) {
	m := newModel(Project{})
	m.tasks = querySample()
	m.filters.Query = ParseQuery(`"docs pass" -status:completed`)
	m.refilter()
	m.openStatusSelector()
	if !m.statusSelected[1] || m.statusSelected[4] {
		t.Fatal("selector should reflect the existing status query")
	}
	m.statusCursor = 4
	m = m.handleStatusKey(tea.KeyMsg{Type: tea.KeySpace})
	m = m.handleStatusKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filters.Query.Raw != `"docs pass"` || len(m.visible) != 1 || m.visible[0].ID != "002" {
		t.Fatalf("query or results changed unexpectedly: %q, %v", m.filters.Query.Raw, m.visible)
	}
	m.openStatusSelector()
	m.statusCursor = 4
	m = m.handleStatusKey(tea.KeyMsg{Type: tea.KeySpace})
	m = m.handleStatusKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.filters.Query.Raw, "status:") || len(m.visible) != 1 {
		t.Fatalf("selection did not preserve the text filter: %q", m.filters.Query.Raw)
	}
	m.openStatusSelector()
	m = m.handleStatusKey(tea.KeyMsg{Type: tea.KeySpace})
	before := m.filters.Query.Raw
	m = m.handleStatusKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.filters.Query.Raw != before {
		t.Fatal("cancel changed the query")
	}
}

func TestStatusSelectorNoneAndFacets(t *testing.T) {
	m := newModel(Project{})
	m.tasks = querySample()
	m.filters.OpenOnly = true
	m.openStatusSelector()
	if !m.statusSelected[1] || m.statusSelected[4] {
		t.Fatal("selector should reflect the open filter")
	}
	for i := range m.statusSelected {
		m.statusSelected[i] = false
	}
	m = m.handleStatusKey(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.visible) != 0 || m.filters.OpenOnly {
		t.Fatal("empty selection should match no tasks and replace the status facet")
	}
	m.openStatusSelector()
	for _, selected := range m.statusSelected {
		if selected {
			t.Fatal("empty selection should be restored")
		}
	}
}

func TestStatusSelectorNavigation(t *testing.T) {
	m := newModel(Project{})
	m.openStatusSelector()
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'j'}},
		{Type: tea.KeyDown},
	} {
		m = m.handleStatusKey(key)
	}
	if m.statusCursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.statusCursor)
	}
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'k'}},
		{Type: tea.KeyUp},
	} {
		m = m.handleStatusKey(key)
	}
	if m.statusCursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.statusCursor)
	}
}
