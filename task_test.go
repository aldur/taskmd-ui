package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTask(t *testing.T) {
	src := `---
id: "042"
title: "Implement user auth"
status: in-progress
priority: high
effort: medium
owner: aldur
phase: core-cli
tags: ["core", "api"]
dependencies: ["001", "015"]
created_at: 2026-04-01
---

# Implement user auth

- [x] design
- [ ] build
`

	task, ok := parseTask([]byte(src))
	if !ok {
		t.Fatal("parseTask rejected a well-formed task")
	}
	if task.ID != "042" || task.Title != "Implement user auth" {
		t.Errorf("id/title = %q/%q", task.ID, task.Title)
	}
	if task.Status != "in-progress" || task.Priority != "high" || task.Effort != "medium" {
		t.Errorf("status/priority/effort = %q/%q/%q", task.Status, task.Priority, task.Effort)
	}
	if task.Owner != "aldur" || task.Phase != "core-cli" {
		t.Errorf("owner/phase = %q/%q", task.Owner, task.Phase)
	}
	if len(task.Tags) != 2 || task.Tags[0] != "core" {
		t.Errorf("tags = %v", task.Tags)
	}
	if len(task.Deps) != 2 || task.Deps[1] != "015" {
		t.Errorf("dependencies = %v", task.Deps)
	}
	if got := task.Created.Format("2006-01-02"); got != "2026-04-01" {
		t.Errorf("created = %s", got)
	}
	if !task.IsOpen() {
		t.Error("in-progress should count as open")
	}
}

// Real projects write ids and dates both quoted and bare; a task that trips
// the decoder would silently vanish from the list.
func TestParseTaskScalarForms(t *testing.T) {
	cases := map[string]struct {
		src     string
		id      string
		tags    []string
		created string
	}{
		"bare id and date": {
			src:     "---\nid: 001\ntitle: Bare\ncreated: 2026-01-02\n---\n",
			id:      "001",
			created: "2026-01-02",
		},
		"prefixed id, flow tags": {
			src:  "---\nid: cli-049\ntitle: Prefixed\ntags: [core, api]\n---\n",
			id:   "cli-049",
			tags: []string{"core", "api"},
		},
		"scalar tag": {
			src:  "---\nid: \"7\"\ntitle: One tag\ntags: docs\n---\n",
			id:   "7",
			tags: []string{"docs"},
		},
		"null fields": {
			src: "---\nid: \"8\"\ntitle: Nulls\ntags:\nowner:\n---\n",
			id:  "8",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			task, ok := parseTask([]byte(tc.src))
			if !ok {
				t.Fatal("parseTask rejected the task")
			}
			if task.ID != tc.id {
				t.Errorf("id = %q, want %q", task.ID, tc.id)
			}
			if len(task.Tags) != len(tc.tags) {
				t.Fatalf("tags = %v, want %v", task.Tags, tc.tags)
			}
			for i, tag := range tc.tags {
				if task.Tags[i] != tag {
					t.Errorf("tags[%d] = %q, want %q", i, task.Tags[i], tag)
				}
			}
			if tc.created != "" && task.Created.Format("2006-01-02") != tc.created {
				t.Errorf("created = %s, want %s", task.Created, tc.created)
			}
		})
	}
}

func TestParseTaskRejectsNonTasks(t *testing.T) {
	cases := map[string]string{
		"no frontmatter":  "# Just a note\n\nnothing to see.\n",
		"unclosed fence":  "---\nid: \"1\"\ntitle: Truncated\n",
		"no id":           "---\ntitle: Anonymous\nstatus: pending\n---\n\nbody\n",
		"empty":           "",
		"malformed yaml":  "---\nid: \"1\"\n  bad: [indent\n---\n",
		"fence not first": "notes\n\n---\nid: \"1\"\ntitle: Late\n---\n",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := parseTask([]byte(src)); ok {
				t.Error("parseTask accepted a non-task")
			}
		})
	}
}

func TestParseTaskFallsBackToHeading(t *testing.T) {
	task, ok := parseTask([]byte("---\nid: \"3\"\nstatus: pending\n---\n\n## Recovered title\n"))
	if !ok {
		t.Fatal("parseTask rejected the task")
	}
	if task.Title != "Recovered title" {
		t.Errorf("title = %q", task.Title)
	}
}

func TestLoadTasks(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("001-first.md", "---\nid: \"001\"\ntitle: First\nstatus: pending\n---\n")
	write("cli/015-scaffolding.md", "---\nid: \"015\"\ntitle: Scaffolding\nstatus: completed\n---\n")
	write("cli/020-grouped.md", "---\nid: \"020\"\ntitle: Grouped\ngroup: explicit\n---\n")
	// Skipped: archived tasks, config templates, and prose without frontmatter
	// — `taskmd list` reports none of them either.
	write("archive/099-old.md", "---\nid: \"099\"\ntitle: Archived\n---\n")
	write(".taskmd/templates/bug.md", "---\nid: \"tpl\"\ntitle: Template\n---\n")
	write("CLAUDE.md", "# Agent notes\n")
	write("notes.txt", "---\nid: \"txt\"\ntitle: Wrong extension\n---\n")

	tasks, err := loadTasks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("loaded %d tasks, want 3: %+v", len(tasks), tasks)
	}

	byID := map[string]Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	if _, ok := byID["099"]; ok {
		t.Error("archived task was loaded")
	}
	if got := byID["015"].Group; got != "cli" {
		t.Errorf("group derived from directory = %q, want cli", got)
	}
	if got := byID["020"].Group; got != "explicit" {
		t.Errorf("explicit group = %q, want explicit", got)
	}
	if got := byID["001"].Rel; got != "001-first.md" {
		t.Errorf("rel = %q", got)
	}
	if byID["001"].Modified.IsZero() {
		t.Error("modified time was not recorded")
	}
}

func TestLoadTasksMissingDir(t *testing.T) {
	if _, err := loadTasks(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("scanning a missing directory should fail")
	}
}
