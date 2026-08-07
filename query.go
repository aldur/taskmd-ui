package main

import (
	"fmt"
	"strings"
	"time"
)

// The `/` prompt takes an expression, not just text. The facet keys handle
// "show me exactly this" in one keypress; everything they cannot say — not
// this, at least this, either of these — is said here.
//
// A query is whitespace-separated terms, all of which must hold:
//
//	parser                  free text: substring over id, title, tags, body…
//	-parser                 …and its negation
//	"record format"         quoted, so the space is part of the text
//	status:blocked          field equals value
//	status!=completed       field does not equal value (also -status:completed)
//	status:pending,blocked  either value
//	status:open             the four unresolved states (also status:done)
//	priority>=high          at least this urgent (priority and effort are ordered)
//	created<2026-06-01      dates compare as dates
//	owner:none              field is unset (owner:any for the opposite)
//	tag:a11y                one of the task's tags
//
// A word with a colon that is not a known field — "orbit query:" out of a
// title — stays free text rather than becoming an error.

type queryOp int

const (
	opEq queryOp = iota
	opGt
	opGe
	opLt
	opLe
)

// queryKind is how a field's values are compared.
type queryKind int

const (
	kindExact     queryKind = iota // status, owner, phase…: equal, case-insensitively
	kindSubstring                  // title, id, file: contained
	kindOrdinal                    // priority, effort: ranked, so >= means "at least"
	kindDate                       // created: parsed, so < means earlier
)

// queryField is one addressable frontmatter field.
type queryField struct {
	name  string
	kind  queryKind
	get   func(Task) []string
	order []string // for kindOrdinal, in the order the list sorts by
	// ascending says which end of `order` is "greater", because the two
	// ordered fields disagree: `priority>=high` means *more* urgent, which is
	// earlier in Priorities, while `effort>=medium` means *more* work, which
	// is later in Efforts.
	ascending bool
	// enum, when set, is the closed set of legal values: a typo in one is
	// reported rather than silently matching nothing.
	enum []string
}

// queryFields maps every name and alias to its field. Aliases exist because
// the keys that cycle these facets are single letters, and muscle memory
// reaches for the same letters when typing.
var queryFields = map[string]queryField{}

func init() {
	register := func(names []string, f queryField) {
		for _, name := range names {
			queryFields[name] = f
		}
	}

	register([]string{"status", "s", "st"}, queryField{
		name: "status", kind: kindExact, enum: Statuses,
		get: func(t Task) []string { return single(t.Status) },
	})
	register([]string{"priority", "prio", "p"}, queryField{
		name: "priority", kind: kindOrdinal, order: Priorities, enum: Priorities,
		get: func(t Task) []string { return single(t.Priority) },
	})
	register([]string{"effort", "e"}, queryField{
		name: "effort", kind: kindOrdinal, order: Efforts, enum: Efforts, ascending: true,
		get: func(t Task) []string { return single(t.Effort) },
	})
	register([]string{"type", "ty", "kind"}, queryField{
		name: "type", kind: kindExact,
		get: func(t Task) []string { return single(t.Type) },
	})
	register([]string{"owner", "o", "who"}, queryField{
		name: "owner", kind: kindExact,
		get: func(t Task) []string { return single(t.Owner) },
	})
	register([]string{"phase", "ph"}, queryField{
		name: "phase", kind: kindExact,
		get: func(t Task) []string { return single(t.Phase) },
	})
	register([]string{"group", "g"}, queryField{
		name: "group", kind: kindExact,
		get: func(t Task) []string { return single(t.Group) },
	})
	register([]string{"tag", "tags", "t"}, queryField{
		name: "tag", kind: kindExact,
		get: func(t Task) []string { return t.Tags },
	})
	register([]string{"dep", "deps", "depends", "dependency", "dependencies"}, queryField{
		name: "dep", kind: kindExact,
		get: func(t Task) []string { return t.Deps },
	})
	register([]string{"touches", "touch", "scope"}, queryField{
		name: "touches", kind: kindExact,
		get: func(t Task) []string { return t.Touches },
	})
	register([]string{"parent"}, queryField{
		name: "parent", kind: kindExact,
		get: func(t Task) []string { return single(t.Parent) },
	})
	register([]string{"id"}, queryField{
		name: "id", kind: kindSubstring,
		get: func(t Task) []string { return single(t.ID) },
	})
	register([]string{"title"}, queryField{
		name: "title", kind: kindSubstring,
		get: func(t Task) []string { return single(t.Title) },
	})
	register([]string{"file", "path"}, queryField{
		name: "file", kind: kindSubstring,
		get: func(t Task) []string { return single(t.Rel) },
	})
	register([]string{"body", "text"}, queryField{
		name: "body", kind: kindSubstring,
		get: func(t Task) []string { return single(t.Body) },
	})
	register([]string{"created", "created_at", "date"}, queryField{
		name: "created", kind: kindDate,
		get: func(t Task) []string { return single(t.CreatedString()) },
	})
}

// CreatedString renders the created date for querying; undated tasks return
// "", which is what `created:none` looks for.
func (t Task) CreatedString() string {
	if t.Created.IsZero() {
		return ""
	}
	return t.Created.Format("2006-01-02")
}

// Sentinel values every field understands.
const (
	sentinelNone = "none"
	sentinelAny  = "any"
)

// statusGroups are the shorthands for a set of statuses. "not done" is the
// question people actually ask, and spelling it `-status:done` beats naming
// both terminal states.
var statusGroups = map[string][]string{
	"open":     {"pending", "in-progress", "in-review", "blocked"},
	"active":   {"pending", "in-progress", "in-review", "blocked"},
	"done":     {"completed", "cancelled"},
	"resolved": {"completed", "cancelled"},
}

// queryTerm is one parsed clause.
type queryTerm struct {
	negate bool
	field  *queryField // nil for free text
	op     queryOp
	values []string
}

// Query is a parsed `/` expression, kept alongside the text it came from so
// the prompt can be restored and the header can echo it.
type Query struct {
	Raw   string
	terms []queryTerm
	// Err is the first complaint, shown in the header. A query that does not
	// parse still filters on the terms that did: the list keeps narrowing
	// while a long expression is half-typed.
	Err string
}

// Empty reports whether the query constrains anything.
func (q Query) Empty() bool { return len(q.terms) == 0 }

// ParseQuery turns prompt text into a Query. It never fails outright — a bad
// term is reported in Err and dropped.
func ParseQuery(raw string) Query {
	query := Query{Raw: raw}
	for _, token := range tokenizeQuery(raw) {
		term, err := parseTerm(token)
		if err != nil {
			if query.Err == "" {
				query.Err = err.Error()
			}
			continue
		}
		query.terms = append(query.terms, term)
	}
	return query
}

// tokenizeQuery splits on whitespace, keeping quoted runs together so a title
// with a space in it can be searched as one term.
func tokenizeQuery(raw string) []string {
	var (
		tokens []string
		buf    strings.Builder
		quote  rune
	)
	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}

	for _, r := range raw {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				buf.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			buf.WriteRune(r)
		}
	}
	flush()
	return tokens
}

// parseTerm reads one token: an optional negation, then either `field op
// value` or free text.
func parseTerm(token string) (queryTerm, error) {
	term := queryTerm{}
	for len(token) > 0 && (token[0] == '-' || token[0] == '!') {
		// Only a leading `-` in front of something else is a negation; a bare
		// `-` or a word starting with one (`-experimental`) is text.
		if len(token) == 1 {
			break
		}
		term.negate = !term.negate
		token = token[1:]
	}
	if token == "" {
		return term, fmt.Errorf("empty term")
	}

	name, op, value, negated, ok := splitTerm(token)
	field, known := queryFields[strings.ToLower(name)]
	if !ok || !known {
		// Free text, negation included: `-docs` hides everything mentioning
		// docs anywhere.
		term.values = []string{strings.ToLower(token)}
		return term, nil
	}

	term.field = &field
	term.op = op
	if negated {
		// `status!=a,b` reads as "neither", so the negation covers the whole
		// term rather than its first value.
		term.negate = !term.negate
	}
	if value == "" {
		return term, fmt.Errorf("%s: needs a value", field.name)
	}
	for _, v := range strings.Split(value, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		term.values = append(term.values, v)
	}
	if len(term.values) == 0 {
		return term, fmt.Errorf("%s: needs a value", field.name)
	}
	return term, validateTerm(term)
}

// splitTerm finds the operator in `field<op>value`. `negated` reports a `!=`,
// which the caller turns into a negation of the whole term.
func splitTerm(token string) (name string, op queryOp, value string, negated, ok bool) {
	for i := 0; i < len(token); i++ {
		switch token[i] {
		case ':', '=':
			return token[:i], opEq, token[i+1:], false, true
		case '!':
			if i+1 < len(token) && token[i+1] == '=' {
				return token[:i], opEq, token[i+2:], true, true
			}
		case '>':
			if i+1 < len(token) && token[i+1] == '=' {
				return token[:i], opGe, token[i+2:], false, true
			}
			return token[:i], opGt, token[i+1:], false, true
		case '<':
			if i+1 < len(token) && token[i+1] == '=' {
				return token[:i], opLe, token[i+2:], false, true
			}
			return token[:i], opLt, token[i+1:], false, true
		}
	}
	return "", opEq, "", false, false
}

// validateTerm rejects values a field cannot take, so a typo reads as an error
// rather than as an empty list.
func validateTerm(term queryTerm) error {
	field := term.field
	for _, value := range term.values {
		if isSentinel(value) {
			continue
		}
		if field.name == "status" && statusGroups[strings.ToLower(value)] != nil {
			continue
		}
		if field.kind == kindDate {
			if parseDate(value).IsZero() {
				return fmt.Errorf("created: %q is not a date (YYYY-MM-DD)", value)
			}
			continue
		}
		if field.enum != nil && !contains(field.enum, strings.ToLower(value)) {
			return fmt.Errorf("%s: %q is not one of %s", field.name, value, strings.Join(field.enum, ", "))
		}
		if term.op != opEq && field.kind != kindOrdinal && field.kind != kindDate {
			return fmt.Errorf("%s: only = and != apply", field.name)
		}
	}
	return nil
}

func isSentinel(value string) bool {
	value = strings.ToLower(value)
	return value == sentinelNone || value == sentinelAny
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// Match reports whether a task satisfies every term.
func (q Query) Match(t Task) bool {
	for _, term := range q.terms {
		if !term.match(t) {
			return false
		}
	}
	return true
}

func (term queryTerm) match(t Task) bool {
	result := term.matches(t)
	if term.negate {
		return !result
	}
	return result
}

func (term queryTerm) matches(t Task) bool {
	if term.field == nil {
		return strings.Contains(strings.ToLower(t.searchable()), term.values[0])
	}

	// Several values on one field are alternatives: `status:pending,blocked`
	// is either of them.
	have := nonEmpty(term.field.get(t))
	for _, want := range term.values {
		if matchValue(*term.field, term.op, have, want) {
			return true
		}
	}
	return false
}

func matchValue(field queryField, op queryOp, have []string, want string) bool {
	switch strings.ToLower(want) {
	case sentinelNone:
		return len(have) == 0
	case sentinelAny:
		return len(have) > 0
	}
	if len(have) == 0 {
		// An unset field is not "less than" anything; it is simply not a
		// candidate, the way `taskmd list --filter` treats it.
		return false
	}

	if field.name == "status" {
		if group, ok := statusGroups[strings.ToLower(want)]; ok {
			for _, status := range group {
				if contains(have, status) {
					return true
				}
			}
			return false
		}
	}

	for _, value := range have {
		switch field.kind {
		case kindSubstring:
			if strings.Contains(strings.ToLower(value), strings.ToLower(want)) {
				return true
			}
		case kindOrdinal:
			got, wanted := rank(field.order, strings.ToLower(value)), rank(field.order, strings.ToLower(want))
			delta := wanted - got
			if field.ascending {
				delta = got - wanted
			}
			if compareOrdinal(op, delta) {
				return true
			}
		case kindDate:
			got, wanted := parseDate(value), parseDate(want)
			if wanted.IsZero() {
				return false
			}
			if compareOrdinal(op, dayDelta(got, wanted)) {
				return true
			}
		default:
			if strings.EqualFold(value, want) {
				return true
			}
		}
	}
	return false
}

// compareOrdinal folds every operator onto the sign of a single difference,
// which each caller orients so that a positive delta means "greater".
func compareOrdinal(op queryOp, delta int) bool {
	switch op {
	case opGt:
		return delta > 0
	case opGe:
		return delta >= 0
	case opLt:
		return delta < 0
	case opLe:
		return delta <= 0
	default:
		return delta == 0
	}
}

// dayDelta is the sign of the difference between two dates, in days.
func dayDelta(got, want time.Time) int {
	switch {
	case got.After(want):
		return 1
	case got.Before(want):
		return -1
	default:
		return 0
	}
}

func nonEmpty(values []string) []string {
	out := values[:0:0]
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}
