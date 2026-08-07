package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Task is one markdown file's frontmatter plus its body.
//
// Tasks are read straight from disk rather than out of `taskmd list --format
// json`, which drops `phase` and `owner`, and `taskmd snapshot`, which drops
// them too — both are filter axes here. Reading the files also keeps a refresh
// free of a subprocess, which is what makes the live reload cheap enough to
// run on a timer. Writes still go through the CLI (see actions.go).
type Task struct {
	ID       string
	Title    string
	Status   string
	Priority string
	Effort   string
	Type     string
	Owner    string
	Phase    string
	Group    string
	Parent   string
	Tags     []string
	Deps     []string
	Touches  []string
	Created  time.Time
	Modified time.Time
	Path     string // absolute, for $EDITOR
	Rel      string // relative to the task directory, for display
	Body     string
}

// Statuses is the status flow from the spec, in workflow order. The order is
// what the status filter and the `1`-`6` shortcuts cycle through.
var Statuses = []string{"pending", "in-progress", "in-review", "blocked", "completed", "cancelled"}

// Priorities runs most to least urgent, which is also the sort order.
var Priorities = []string{"critical", "high", "medium", "low"}

// Efforts runs smallest first.
var Efforts = []string{"small", "medium", "large"}

// openStatuses are the states a task still needs work in — what the `open`
// filter preset keeps.
var openStatuses = map[string]bool{
	"pending":     true,
	"in-progress": true,
	"in-review":   true,
	"blocked":     true,
}

// IsOpen reports whether the task is unresolved.
func (t Task) IsOpen() bool { return openStatuses[t.Status] }

// loadTasks walks the task directory and parses every markdown file that
// carries frontmatter with an id. Files without it — `CLAUDE.md`,
// `TASKMD_SPEC.md`, stray notes — are skipped silently, as are unreadable
// ones: a single bad file should not empty the list.
func loadTasks(dir string) ([]Task, error) {
	var tasks []Task

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// The task directory itself being unreadable is worth reporting —
			// it usually means the project was not found. Anything below it is
			// skipped instead, so one bad subdirectory cannot empty the list.
			if path == dir {
				return err
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			// `taskmd list` does not report archived tasks, and neither does
			// this; dot-directories hold config (`.taskmd/templates`), not
			// tasks.
			name := d.Name()
			if path != dir && (name == "archive" || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		task, ok := parseTask(data)
		if !ok {
			return nil
		}

		task.Path = path
		if rel, err := filepath.Rel(dir, path); err == nil {
			task.Rel = rel
		} else {
			task.Rel = filepath.Base(path)
		}
		if task.Group == "" {
			// Spec: explicit `group` wins, else the parent directory name.
			if parent := filepath.Dir(task.Rel); parent != "." {
				task.Group = filepath.Base(parent)
			}
		}
		if info, err := d.Info(); err == nil {
			task.Modified = info.ModTime()
		}
		tasks = append(tasks, task)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// A stable base order, so equal keys under any sort stay put between
	// reloads instead of shuffling under the cursor.
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Rel < tasks[j].Rel })
	return tasks, nil
}

// frontmatter is the YAML block, held in scalar-tolerant types: ids like `001`
// and dates are written both quoted and bare across real projects, and a task
// that trips the decoder would otherwise vanish from the list.
type frontmatter struct {
	ID        flexStr  `yaml:"id"`
	Title     flexStr  `yaml:"title"`
	Status    flexStr  `yaml:"status"`
	Priority  flexStr  `yaml:"priority"`
	Effort    flexStr  `yaml:"effort"`
	Type      flexStr  `yaml:"type"`
	Owner     flexStr  `yaml:"owner"`
	Phase     flexStr  `yaml:"phase"`
	Group     flexStr  `yaml:"group"`
	Parent    flexStr  `yaml:"parent"`
	Tags      flexList `yaml:"tags"`
	Deps      flexList `yaml:"dependencies"`
	Touches   flexList `yaml:"touches"`
	CreatedAt flexStr  `yaml:"created_at"`
	Created   flexStr  `yaml:"created"`
}

// parseTask splits the frontmatter off the body and decodes it. The second
// return is false for anything that is not a task file.
func parseTask(data []byte) (Task, bool) {
	front, body, ok := splitFrontmatter(string(data))
	if !ok {
		return Task{}, false
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return Task{}, false
	}
	if fm.ID == "" {
		// `id` is the one field the spec requires; without it there is nothing
		// to address the task by, and `taskmd` would not list it either.
		return Task{}, false
	}

	task := Task{
		ID:       string(fm.ID),
		Title:    string(fm.Title),
		Status:   strings.ToLower(string(fm.Status)),
		Priority: strings.ToLower(string(fm.Priority)),
		Effort:   strings.ToLower(string(fm.Effort)),
		Type:     strings.ToLower(string(fm.Type)),
		Owner:    string(fm.Owner),
		Phase:    string(fm.Phase),
		Group:    string(fm.Group),
		Parent:   string(fm.Parent),
		Tags:     fm.Tags,
		Deps:     fm.Deps,
		Touches:  fm.Touches,
		Body:     strings.TrimSpace(body),
	}
	if task.Title == "" {
		task.Title = firstHeading(task.Body)
	}
	created := fm.CreatedAt
	if created == "" {
		created = fm.Created
	}
	task.Created = parseDate(string(created))
	return task, true
}

// splitFrontmatter returns the YAML block and the markdown that follows a
// leading `---` fence.
func splitFrontmatter(text string) (front, body string, ok bool) {
	// A UTF-8 BOM ahead of the fence is common enough to be worth tolerating.
	text = strings.TrimPrefix(text, "\ufeff")

	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		switch strings.TrimSpace(lines[i]) {
		case "---", "...":
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), true
		}
	}
	return "", "", false
}

// firstHeading is the fallback title for a task whose frontmatter omits one.
func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return "(untitled)"
}

// dateLayouts covers what the spec allows (`YYYY-MM-DD`) plus the timestamps
// tools write back.
var dateLayouts = []string{
	"2006-01-02",
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

func parseDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

// flexStr decodes any scalar — quoted, bare, numeric, dated — as its literal
// text, so `id: 001` and `id: "001"` land on the same string.
type flexStr string

func (f *flexStr) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag != "!!null" {
		*f = flexStr(strings.TrimSpace(node.Value))
	}
	return nil
}

// flexList decodes a sequence of scalars, and accepts a bare scalar as a
// one-element list.
type flexList []string

func (f *flexList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind == yaml.ScalarNode && item.Tag != "!!null" {
				if value := strings.TrimSpace(item.Value); value != "" {
					out = append(out, value)
				}
			}
		}
		*f = out
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			return nil
		}
		if value := strings.TrimSpace(node.Value); value != "" {
			*f = []string{value}
		}
	}
	return nil
}
