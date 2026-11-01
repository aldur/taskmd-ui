package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) openStatusSelector() {
	m.selectingStatus = true
	m.statusCursor = 0
	for i, status := range Statuses {
		task := Task{Status: status}
		selected := (!m.filters.OpenOnly || task.IsOpen()) && matchFacet(m.filters.Status, status)
		for _, term := range m.filters.Query.terms {
			if term.field != nil && term.field.name == "status" && !term.match(task) {
				selected = false
			}
		}
		m.statusSelected[i] = selected
	}
}

func (m model) handleStatusKey(msg tea.KeyMsg) model {
	switch msg.String() {
	case "esc", "q":
		m.selectingStatus = false
	case "j", "down":
		m.statusCursor = minInt(m.statusCursor+1, len(Statuses)-1)
	case "k", "up":
		m.statusCursor = maxInt(m.statusCursor-1, 0)
	case " ":
		m.statusSelected[m.statusCursor] = !m.statusSelected[m.statusCursor]
	case "enter":
		raw := withoutStatusTerms(m.filters.Query.Raw)
		var selected, excluded []string
		for i, status := range Statuses {
			if m.statusSelected[i] {
				selected = append(selected, status)
			} else {
				excluded = append(excluded, status)
			}
		}
		switch {
		case len(selected) == 0:
			raw = strings.TrimSpace(raw + " status:none -status:none")
		case len(excluded) > 0:
			raw = strings.TrimSpace(raw + " status:" + strings.Join(selected, ","))
		}
		m.filters.Query = ParseQuery(raw)
		m.filters.Status = anyValue
		m.filters.OpenOnly = false
		m.search.SetValue(raw)
		m.selectingStatus = false
		m.refilter()
	}
	return m
}

// Keep the original spelling and quotes of terms that do not target status.
func withoutStatusTerms(raw string) string {
	var kept []string
	start := -1
	var quote byte
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || (quote == 0 && (raw[i] == ' ' || raw[i] == '\t')) {
			if start >= 0 {
				original := raw[start:i]
				tokens := tokenizeQuery(original)
				if len(tokens) != 1 {
					kept = append(kept, original)
				} else if term, err := parseTerm(tokens[0]); err != nil || term.field == nil || term.field.name != "status" {
					kept = append(kept, original)
				}
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
		if quote != 0 && raw[i] == quote {
			quote = 0
		} else if quote == 0 && (raw[i] == '\'' || raw[i] == '"') {
			quote = raw[i]
		}
	}
	return strings.Join(kept, " ")
}

func (m model) statusSelectorView() string {
	lines := []string{"Select statuses", "j/k or ↑/↓ move · space toggle · enter apply · esc cancel"}
	for i, status := range Statuses {
		cursor, check := "  ", "[ ]"
		if i == m.statusCursor {
			cursor = "> "
		}
		if m.statusSelected[i] {
			check = "[x]"
		}
		lines = append(lines, cursor+check+" "+status)
	}
	if len(lines) > m.height {
		start := minInt(maxInt(0, m.statusCursor+3-m.height), len(lines)-m.height)
		lines = lines[start : start+m.height]
	}
	for i := range lines {
		lines[i] = truncate(lines[i], m.width)
	}
	return strings.Join(lines, "\n")
}
