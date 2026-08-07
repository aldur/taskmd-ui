package main

import (
	"strings"
	"testing"
)

// querySample spans the axes the expression language addresses: every status,
// unset fields, multiple tags, and dates on either side of a boundary.
func querySample() []Task {
	return []Task{
		{ID: "001", Title: "Parser rewrite", Status: "pending", Priority: "high", Effort: "large", Owner: "aldur", Phase: "core", Tags: []string{"cli", "core"}, Created: date("2026-01-05")},
		{ID: "002", Title: "Docs pass", Status: "in-progress", Priority: "low", Effort: "small", Tags: []string{"docs"}, Created: date("2026-07-01")},
		{ID: "003", Title: "Board view", Status: "in-review", Priority: "critical", Effort: "medium", Owner: "mira", Tags: []string{"web", "ui"}, Body: "renders a kanban board"},
		{ID: "004", Title: "Dark mode", Status: "blocked", Priority: "medium", Owner: "mira", Phase: "web", Tags: []string{"web"}, Created: date("2026-06-01")},
		{ID: "005", Title: "Old idea", Status: "cancelled", Priority: "low", Created: date("2025-12-01")},
		{ID: "006", Title: "Shipped thing", Status: "completed", Priority: "high", Effort: "medium", Owner: "aldur", Deps: []string{"001"}, Created: date("2026-03-15")},
	}
}

func matching(t *testing.T, expr string) string {
	t.Helper()
	query := ParseQuery(expr)
	var out []string
	for _, task := range querySample() {
		if query.Match(task) {
			out = append(out, task.ID)
		}
	}
	return strings.Join(out, ",")
}

func TestQueryEquality(t *testing.T) {
	cases := map[string]string{
		"status:blocked":         "004",
		"status=blocked":         "004",
		"owner:mira":             "003,004",
		"tag:web":                "003,004",
		"phase:core":             "001",
		"dep:001":                "006",
		"id:00":                  "001,002,003,004,005,006",
		"id:4":                   "004",
		"title:dark":             "004",
		"s:pending":              "001", // alias
		"prio:critical":          "003",
		"status:pending,blocked": "001,004", // either
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			if got := matching(t, expr); got != want {
				t.Errorf("= %s, want %s", got, want)
			}
		})
	}
}

// The gap the facet keys cannot fill: everything that is not X.
func TestQueryNegation(t *testing.T) {
	cases := map[string]string{
		"-status:completed":               "001,002,003,004,005",
		"status!=completed":               "001,002,003,004,005",
		"!status:completed":               "001,002,003,004,005",
		"-status:done":                    "001,002,003,004",
		"status!=completed,cancelled":     "001,002,003,004",
		"-owner:mira":                     "001,002,005,006",
		"-tag:web":                        "001,002,005,006",
		"-parser":                         "002,003,004,005,006",
		"-status:completed -priority:low": "001,003,004",
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			if got := matching(t, expr); got != want {
				t.Errorf("= %s, want %s", got, want)
			}
		})
	}
}

// Priority counts down (critical is greatest) and effort counts up, so the
// same operator has to mean "more urgent" on one and "more work" on the other.
func TestQueryOrdinal(t *testing.T) {
	cases := map[string]string{
		"priority>=high":  "001,003,006",
		"priority>high":   "003",
		"priority<=low":   "002,005",
		"priority<medium": "002,005",
		"effort>=medium":  "001,003,006",
		"effort>medium":   "001",
		"effort<=small":   "002",
		"effort<medium":   "002",
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			if got := matching(t, expr); got != want {
				t.Errorf("= %s, want %s", got, want)
			}
		})
	}
}

func TestQueryDates(t *testing.T) {
	cases := map[string]string{
		"created>=2026-06-01": "002,004",
		"created>2026-06-01":  "002",
		"created<2026-01-01":  "005",
		"created:2026-06-01":  "004",
		"created:none":        "003",
		"created:any":         "001,002,004,005,006",
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			if got := matching(t, expr); got != want {
				t.Errorf("= %s, want %s", got, want)
			}
		})
	}
}

func TestQuerySentinels(t *testing.T) {
	cases := map[string]string{
		"owner:none":  "002,005",
		"owner:any":   "001,003,004,006",
		"-owner:none": "001,003,004,006",
		"phase:none":  "002,003,005,006",
		"tag:none":    "005,006",
		"effort:none": "004,005",
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			if got := matching(t, expr); got != want {
				t.Errorf("= %s, want %s", got, want)
			}
		})
	}
}

func TestQueryStatusGroups(t *testing.T) {
	cases := map[string]string{
		"status:open":     "001,002,003,004",
		"status:active":   "001,002,003,004",
		"status:done":     "005,006",
		"status:resolved": "005,006",
		"-status:open":    "005,006",
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			if got := matching(t, expr); got != want {
				t.Errorf("= %s, want %s", got, want)
			}
		})
	}
}

func TestQueryFreeTextAndCombinations(t *testing.T) {
	cases := map[string]string{
		"":                              "001,002,003,004,005,006",
		"   ":                           "001,002,003,004,005,006",
		"kanban":                        "003", // body
		"PARSER":                        "001", // case-insensitive
		"\"parser rewrite\"":            "001", // quoted, space included
		"'dark mode'":                   "004",
		"parser rewrite":                "001", // terms are ANDed
		"status:open priority>=high":    "001,003",
		"owner:aldur -status:completed": "001",
		"tag:web status!=blocked":       "003",
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			if got := matching(t, expr); got != want {
				t.Errorf("= %s, want %s", got, want)
			}
		})
	}
}

// A colon in a title is not a field, and should not become one.
func TestQueryUnknownFieldIsText(t *testing.T) {
	tasks := []Task{{ID: "1", Title: "orbit query: filters and projections"}, {ID: "2", Title: "something else"}}
	query := ParseQuery("query:")
	if query.Err != "" {
		t.Errorf("unknown field should not be an error, got %q", query.Err)
	}
	if !query.Match(tasks[0]) || query.Match(tasks[1]) {
		t.Error("an unknown field should fall back to free text")
	}
}

// A typo in a closed set is reported, rather than silently matching nothing.
func TestQueryReportsBadValues(t *testing.T) {
	cases := []string{
		"status:done-ish",
		"priority:urgent",
		"effort:enormous",
		"created:soon",
		"status:",
	}
	for _, expr := range cases {
		t.Run(expr, func(t *testing.T) {
			if query := ParseQuery(expr); query.Err == "" {
				t.Error("expected a complaint")
			}
		})
	}
}

// A half-typed expression keeps filtering on the part that parsed, which is
// what makes typing one feel live.
func TestQueryPartialStillFilters(t *testing.T) {
	query := ParseQuery("owner:mira status:xyz")
	if query.Err == "" {
		t.Error("the bad term should be reported")
	}
	if got := matching(t, "owner:mira status:xyz"); got != "003,004" {
		t.Errorf("= %s, want the good term still applied", got)
	}
}

func TestQueryEmpty(t *testing.T) {
	if !ParseQuery("").Empty() {
		t.Error("no text is no terms")
	}
	if ParseQuery("status:open").Empty() {
		t.Error("a parsed term is a term")
	}
	// A term that failed to parse constrains nothing, so it does not count.
	if !ParseQuery("status:nope").Empty() {
		t.Error("a rejected term should leave the query empty")
	}
}
