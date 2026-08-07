package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Project is the task tree the TUI is pointed at: the directory holding
// `.taskmd.yaml` (what `taskmd` itself treats as the project root, and the cwd
// its subcommands are run from) and the resolved directory tasks live in.
type Project struct {
	Root    string
	TaskDir string
	Phases  []Phase
}

// Phase mirrors an entry of the `phases:` list in `.taskmd.yaml`. Only the
// fields the TUI shows or filters on are read.
type Phase struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// Key returns what a task's `phase:` field is matched against: the phase id,
// falling back to its name, as the spec describes.
func (p Phase) Key() string {
	if p.ID != "" {
		return p.ID
	}
	return p.Name
}

type projectConfig struct {
	Dir     string  `yaml:"dir"`
	TaskDir string  `yaml:"task_dir"`
	Phases  []Phase `yaml:"phases"`
}

const configName = ".taskmd.yaml"

// discoverProject resolves the project the same way the `taskmd` CLI does:
// walk up from the working directory for `.taskmd.yaml` and take its `dir`.
// An explicit override skips the lookup for the task directory but still lets
// a config above it supply the phase list.
func discoverProject(override string) (Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Project{}, err
	}

	proj := Project{Root: cwd}
	if root, cfg, ok := findConfig(cwd); ok {
		proj.Root = root
		proj.Phases = cfg.Phases
		dir := cfg.Dir
		if dir == "" {
			dir = cfg.TaskDir
		}
		if dir != "" {
			proj.TaskDir = absFrom(root, dir)
		}
	}

	if override != "" {
		proj.TaskDir = absFrom(cwd, override)
		// -d can point into a project we are not standing in; that project's
		// config is the one that names its phases and roots its `taskmd`
		// calls. Pointed inside the project we are already in, this finds the
		// same file again.
		if root, cfg, ok := findConfig(proj.TaskDir); ok {
			proj.Root = root
			proj.Phases = cfg.Phases
		}
	}
	if proj.TaskDir == "" {
		// No config to point the way: a `tasks/` beside us is the layout
		// `taskmd init` creates. Falling back to the working directory itself
		// is deliberately not done — every reload walks the whole tree, and
		// starting that on a home directory by mistake is worse than saying
		// no. `-d` is there for a bare pile of task files.
		dir := filepath.Join(proj.Root, "tasks")
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return proj, fmt.Errorf("no %s or tasks/ above %s — run `taskmd init`, or point -d at a task directory", configName, cwd)
		}
		proj.TaskDir = dir
	}

	if info, err := os.Stat(proj.TaskDir); err != nil {
		return proj, err
	} else if !info.IsDir() {
		return proj, &os.PathError{Op: "scan", Path: proj.TaskDir, Err: os.ErrInvalid}
	}
	return proj, nil
}

// findConfig walks from dir to the filesystem root looking for a readable
// `.taskmd.yaml`. A malformed one is treated as absent rather than fatal: the
// TUI only reads it for the task directory and phase names, both of which have
// working fallbacks.
func findConfig(dir string) (string, projectConfig, bool) {
	for {
		path := filepath.Join(dir, configName)
		if data, err := os.ReadFile(path); err == nil {
			var cfg projectConfig
			if err := yaml.Unmarshal(data, &cfg); err == nil {
				return dir, cfg, true
			}
			return dir, projectConfig{}, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", projectConfig{}, false
		}
		dir = parent
	}
}

func absFrom(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}

// phaseLabel renders a task's raw `phase` value using the display name the
// config gives it, so the filter line reads like the project talks.
func (p Project) phaseLabel(key string) string {
	for _, ph := range p.Phases {
		if ph.Key() == key && ph.Name != "" {
			return ph.Name
		}
	}
	return key
}
