package main

import (
	"strings"
	"testing"
	"time"
)

func sample() []Task {
	return []Task{
		{ID: "001", Title: "Parser rewrite", Status: "in-progress", Priority: "high", Effort: "large", Owner: "aldur", Phase: "core", Tags: []string{"cli", "core"}, Created: date("2026-01-05")},
		{ID: "002", Title: "Docs pass", Status: "pending", Priority: "low", Effort: "small", Tags: []string{"docs"}, Created: date("2026-03-01")},
		{ID: "010", Title: "Ship board view", Status: "completed", Priority: "critical", Owner: "aldur", Phase: "web", Body: "renders a kanban board"},
		{ID: "011", Title: "Cancelled idea", Status: "cancelled", Priority: "medium"},
		{ID: "012", Title: "Blocked on upstream", Status: "blocked", Priority: "high", Tags: []string{"cli"}},
	}
}

func date(value string) time.Time {
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(err)
	}
	return t
}

func ids(tasks []Task) string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.ID
	}
	return strings.Join(out, ",")
}

func TestFilterOpenOnly(t *testing.T) {
	got := Apply(sample(), Filters{OpenOnly: true}, Sort{Field: SortID})
	if want := "001,002,012"; ids(got) != want {
		t.Errorf("open tasks = %s, want %s", ids(got), want)
	}
}

func TestFilterFacets(t *testing.T) {
	cases := map[string]struct {
		filters Filters
		want    string
	}{
		"status":       {Filters{Status: "blocked"}, "012"},
		"priority":     {Filters{Priority: "high"}, "001,012"},
		"tag":          {Filters{Tag: "cli"}, "001,012"},
		"owner":        {Filters{Owner: "aldur"}, "001,010"},
		"phase":        {Filters{Phase: "web"}, "010"},
		"no owner":     {Filters{Owner: noneValue}, "002,011,012"},
		"no tag":       {Filters{Tag: noneValue}, "010,011"},
		"combined":     {Filters{OpenOnly: true, Priority: "high"}, "001,012"},
		"case folding": {Filters{Status: "IN-PROGRESS"}, "001"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ids(Apply(sample(), tc.filters, Sort{Field: SortID})); got != tc.want {
				t.Errorf("= %s, want %s", got, tc.want)
			}
		})
	}
}

func TestFilterQuery(t *testing.T) {
	cases := map[string]struct {
		query string
		want  string
	}{
		"title substring":  {"parser", "001"},
		"case insensitive": {"DOCS", "002"},
		"matches body":     {"kanban", "010"},
		"matches id":       {"012", "012"},
		"matches tag":      {"cli", "001,012"},
		"terms are anded":  {"board ship", "010"},
		"no match":         {"nothing here", ""},
		"empty keeps all":  {"", "001,002,010,011,012"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ids(Apply(sample(), Filters{Query: ParseQuery(tc.query)}, Sort{Field: SortID})); got != tc.want {
				t.Errorf("query %q = %s, want %s", tc.query, got, tc.want)
			}
		})
	}
}

func TestSortOrders(t *testing.T) {
	cases := map[string]struct {
		order Sort
		want  string
	}{
		"priority":         {Sort{Field: SortPriority}, "010,001,012,011,002"},
		"priority reverse": {Sort{Field: SortPriority, Reverse: true}, "002,011,001,012,010"},
		"status":           {Sort{Field: SortStatus}, "002,001,012,010,011"},
		"id":               {Sort{Field: SortID}, "001,002,010,011,012"},
		"title":            {Sort{Field: SortTitle}, "012,011,002,001,010"},
		// Newest first, and the undated tasks after everything dated.
		"created": {Sort{Field: SortCreated}, "002,001,010,011,012"},
		// small, medium, large, then the tasks with no effort at all.
		"effort": {Sort{Field: SortEffort}, "002,001,010,011,012"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ids(Apply(sample(), Filters{}, tc.order)); got != tc.want {
				t.Errorf("= %s, want %s", got, tc.want)
			}
		})
	}
}

// Numeric ids should not sort as text, or task 9 lands after task 10.
func TestSortIDIsNumeric(t *testing.T) {
	tasks := []Task{{ID: "10"}, {ID: "9"}, {ID: "cli-2"}, {ID: "cli-10"}, {ID: "2"}}
	if got, want := ids(Apply(tasks, Filters{}, Sort{Field: SortID})), "2,9,10,cli-2,cli-10"; got != want {
		t.Errorf("= %s, want %s", got, want)
	}
}

func TestFacetValues(t *testing.T) {
	values := facetValues(sample(), func(t Task) []string { return single(t.Owner) })
	if want := 3; len(values) != want { // any, aldur, none
		t.Fatalf("owner facet = %q, want %d entries", values, want)
	}
	if values[0] != anyValue || values[len(values)-1] != noneValue {
		t.Errorf("facet should run from any to none: %q", values)
	}

	tags := facetValues(sample(), func(t Task) []string { return t.Tags })
	if want := "|cli|core|docs|\x00none"; strings.Join(tags, "|") != want {
		t.Errorf("tag facet = %q, want %q", strings.Join(tags, "|"), want)
	}
}

func TestCycleWraps(t *testing.T) {
	values := []string{anyValue, "a", "b"}
	if got := cycle(values, anyValue, false); got != "a" {
		t.Errorf("forward from any = %q", got)
	}
	if got := cycle(values, "b", false); got != anyValue {
		t.Errorf("forward from the end should wrap, got %q", got)
	}
	if got := cycle(values, anyValue, true); got != "b" {
		t.Errorf("backward from any should wrap, got %q", got)
	}
	// A value whose last task was just retagged is no longer in the cycle.
	if got := cycle(values, "gone", false); got != "a" {
		t.Errorf("stale value should restart the cycle, got %q", got)
	}
}

func TestSortNextCyclesEveryField(t *testing.T) {
	order := Sort{Field: sortFields[0]}
	seen := map[SortField]bool{}
	for range sortFields {
		if seen[order.Field] {
			t.Fatalf("%s repeated before the cycle closed", order.Field)
		}
		seen[order.Field] = true
		order = order.Next()
	}
	if order.Field != sortFields[0] {
		t.Errorf("cycle ended on %s, want %s", order.Field, sortFields[0])
	}
}

func TestAdvanceStatus(t *testing.T) {
	cases := map[string]string{
		"pending":     "in-progress",
		"in-progress": "in-review",
		"in-review":   "completed",
		"blocked":     "in-progress",
		"completed":   "pending",
		"cancelled":   "pending",
		"":            "in-progress",
	}
	for from, want := range cases {
		if got := advanceStatus(from); got != want {
			t.Errorf("advanceStatus(%q) = %q, want %q", from, got, want)
		}
	}
}

func TestFiltersActive(t *testing.T) {
	if (Filters{}).Active() {
		t.Error("an empty filter set is not active")
	}
	if !(Filters{OpenOnly: true}).Active() {
		t.Error("open-only is a filter")
	}
	if !(Filters{Query: ParseQuery("x")}).Active() {
		t.Error("a query is a filter")
	}
	// Text that parses to nothing constrains nothing.
	if (Filters{Query: ParseQuery("   ")}).Active() {
		t.Error("a blank query is not a filter")
	}
}
