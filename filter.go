package main

import (
	"sort"
	"strings"
)

// anyValue is the sentinel a facet holds when it is not filtering. It is not a
// legal frontmatter value, so it cannot collide with a real one.
const anyValue = ""

// noneValue matches tasks that leave the field empty — "which of these has
// nobody on it" is as useful a question as "which are Adriano's".
const noneValue = "\x00none"

// Filters is the live query: the facets cycle one keypress at a time, the
// query is typed. Everything is applied together, all conditions ANDed.
type Filters struct {
	Query    Query
	OpenOnly bool
	Status   string
	Priority string
	Tag      string
	Phase    string
	Owner    string
	Group    string
	Type     string
}

// Active reports whether anything is being filtered out. Spelled out rather
// than compared against a zero value: Filters is no longer comparable now that
// it carries a parsed query.
func (f Filters) Active() bool {
	return f.OpenOnly ||
		f.Status != anyValue ||
		f.Priority != anyValue ||
		f.Tag != anyValue ||
		f.Phase != anyValue ||
		f.Owner != anyValue ||
		f.Group != anyValue ||
		f.Type != anyValue ||
		!f.Query.Empty()
}

// Match reports whether a task survives the current filters.
func (f Filters) Match(t Task) bool {
	if f.OpenOnly && !t.IsOpen() {
		return false
	}
	if !matchFacet(f.Status, t.Status) ||
		!matchFacet(f.Priority, t.Priority) ||
		!matchFacet(f.Phase, t.Phase) ||
		!matchFacet(f.Owner, t.Owner) ||
		!matchFacet(f.Group, t.Group) ||
		!matchFacet(f.Type, t.Type) {
		return false
	}
	if !matchTag(f.Tag, t.Tags) {
		return false
	}
	return f.Query.Match(t)
}

func matchFacet(want, have string) bool {
	switch want {
	case anyValue:
		return true
	case noneValue:
		return have == ""
	default:
		return strings.EqualFold(want, have)
	}
}

func matchTag(want string, tags []string) bool {
	switch want {
	case anyValue:
		return true
	case noneValue:
		return len(tags) == 0
	}
	for _, tag := range tags {
		if strings.EqualFold(want, tag) {
			return true
		}
	}
	return false
}

// searchable is the haystack a query's free-text terms run against. Substring
// rather than fuzzy on purpose: typing `api` should not also surface `a-p-i`
// spread across a body. The body is included so a half-remembered detail finds
// its task. See query.go for the rest of the expression language.
func (t Task) searchable() string {
	var b strings.Builder
	for _, part := range []string{t.ID, t.Title, t.Status, t.Priority, t.Type, t.Owner, t.Phase, t.Group, t.Rel, t.Body} {
		b.WriteString(part)
		b.WriteByte(' ')
	}
	for _, tag := range t.Tags {
		b.WriteString(tag)
		b.WriteByte(' ')
	}
	return b.String()
}

// SortField is the column the list is ordered by.
type SortField int

const (
	SortPriority SortField = iota
	SortStatus
	SortID
	SortCreated
	SortModified
	SortEffort
	SortTitle
)

// sortFields is the cycle order of the `s` key.
var sortFields = []SortField{SortPriority, SortStatus, SortID, SortCreated, SortModified, SortEffort, SortTitle}

func (s SortField) String() string {
	switch s {
	case SortPriority:
		return "priority"
	case SortStatus:
		return "status"
	case SortID:
		return "id"
	case SortCreated:
		return "created"
	case SortModified:
		return "modified"
	case SortEffort:
		return "effort"
	case SortTitle:
		return "title"
	}
	return "?"
}

// Sort is the ordering the list is displayed in. Descending here means
// "reversed": the natural order of every field is the useful one — most urgent
// priority, earliest workflow state, newest timestamp — so ascending is the
// default in each case and `S` flips it.
type Sort struct {
	Field   SortField
	Reverse bool
}

// Next advances to the next sort field, keeping the direction.
func (s Sort) Next() Sort {
	for i, field := range sortFields {
		if field == s.Field {
			s.Field = sortFields[(i+1)%len(sortFields)]
			return s
		}
	}
	s.Field = sortFields[0]
	return s
}

// Apply filters and sorts in one pass, returning a new slice.
func Apply(tasks []Task, filters Filters, order Sort) []Task {
	out := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if filters.Match(t) {
			out = append(out, t)
		}
	}

	// SliceStable, so tasks the key cannot separate keep the file order
	// loadTasks put them in and the list does not churn on reload.
	sort.SliceStable(out, func(i, j int) bool {
		less := lessBy(order.Field, out[i], out[j])
		if order.Reverse {
			return lessBy(order.Field, out[j], out[i])
		}
		return less
	})
	return out
}

func lessBy(field SortField, a, b Task) bool {
	switch field {
	case SortPriority:
		return rank(Priorities, a.Priority) < rank(Priorities, b.Priority)
	case SortStatus:
		return rank(Statuses, a.Status) < rank(Statuses, b.Status)
	case SortEffort:
		return rank(Efforts, a.Effort) < rank(Efforts, b.Effort)
	case SortCreated:
		if a.Created.Equal(b.Created) {
			return false
		}
		// Newest first, and undated tasks last rather than in 0001 AD.
		if a.Created.IsZero() != b.Created.IsZero() {
			return b.Created.IsZero()
		}
		return a.Created.After(b.Created)
	case SortModified:
		return a.Modified.After(b.Modified)
	case SortTitle:
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	default:
		return lessID(a.ID, b.ID)
	}
}

// rank orders a value by its position in the canonical list, sorting anything
// unset or unrecognised last instead of alphabetically among the known values.
func rank(order []string, value string) int {
	for i, v := range order {
		if v == value {
			return i
		}
	}
	return len(order)
}

// lessID compares ids so `9` sorts before `10` while prefixed ids (`cli-049`)
// still group by prefix.
func lessID(a, b string) bool {
	ap, an, aok := splitID(a)
	bp, bn, bok := splitID(b)
	if aok && bok && ap == bp && an != bn {
		return an < bn
	}
	return a < b
}

// splitID peels a trailing run of digits off an id.
func splitID(id string) (prefix string, number int, ok bool) {
	end := len(id)
	for end > 0 && id[end-1] >= '0' && id[end-1] <= '9' {
		end--
	}
	digits := id[end:]
	if digits == "" || len(digits) > 18 {
		return id, 0, false
	}
	for _, c := range digits {
		number = number*10 + int(c-'0')
	}
	return id[:end], number, true
}

// facetValues collects the distinct values a field takes across the loaded
// tasks, so the cycling keys only ever offer choices that match something.
// `any` leads, and `none` follows when some task leaves the field empty.
func facetValues(tasks []Task, get func(Task) []string) []string {
	seen := map[string]bool{}
	var values []string
	hasNone := false
	for _, t := range tasks {
		items := get(t)
		if len(items) == 0 {
			hasNone = true
			continue
		}
		for _, item := range items {
			if item == "" {
				hasNone = true
				continue
			}
			if !seen[item] {
				seen[item] = true
				values = append(values, item)
			}
		}
	}
	sort.Strings(values)

	out := []string{anyValue}
	out = append(out, values...)
	if hasNone {
		out = append(out, noneValue)
	}
	return out
}

// cycle steps a facet to the next value it can take, wrapping around. A value
// that no longer exists — its last task was just retagged — restarts the cycle.
func cycle(values []string, current string, back bool) string {
	if len(values) == 0 {
		return anyValue
	}
	index := 0
	for i, v := range values {
		if v == current {
			index = i
			break
		}
	}
	if back {
		return values[(index-1+len(values))%len(values)]
	}
	return values[(index+1)%len(values)]
}

// single wraps a scalar field for facetValues.
func single(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}
