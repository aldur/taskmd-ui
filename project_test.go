package main

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir points the process at dir for the duration of the test; discovery
// starts from the working directory, the way the CLI's does.
func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The config can sit any number of directories above the one you run in —
// `taskmd list` works from `tasks/`, so this has to as well.
func TestDiscoverProjectWalksUp(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, configName), "dir: ./tasks\n")
	writeFile(t, filepath.Join(root, "tasks", "cli", "001-a.md"), "---\nid: \"1\"\n---\n")
	chdir(t, filepath.Join(root, "tasks", "cli"))

	proj, err := discoverProject("")
	if err != nil {
		t.Fatal(err)
	}
	if want := resolve(t, root); proj.Root != want {
		t.Errorf("root = %q, want %q", proj.Root, want)
	}
	if want := resolve(t, filepath.Join(root, "tasks")); proj.TaskDir != want {
		t.Errorf("task dir = %q, want %q", proj.TaskDir, want)
	}
}

func TestDiscoverProjectReadsPhases(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, configName), `dir: ./tasks
phases:
  - id: core-cli
    name: "Core CLI"
  - name: "Web Dashboard"
`)
	writeFile(t, filepath.Join(root, "tasks", "001-a.md"), "---\nid: \"1\"\n---\n")
	chdir(t, root)

	proj, err := discoverProject("")
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.Phases) != 2 {
		t.Fatalf("phases = %+v", proj.Phases)
	}
	// A task's `phase` matches the id, falling back to the name.
	if got := proj.phaseLabel("core-cli"); got != "Core CLI" {
		t.Errorf("phaseLabel(core-cli) = %q", got)
	}
	if got := proj.phaseLabel("Web Dashboard"); got != "Web Dashboard" {
		t.Errorf("phaseLabel(Web Dashboard) = %q", got)
	}
	// An unknown phase still displays as itself rather than disappearing.
	if got := proj.phaseLabel("v0.9"); got != "v0.9" {
		t.Errorf("phaseLabel(v0.9) = %q", got)
	}
}

func TestDiscoverProjectOverride(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, configName), "dir: ./tasks\n")
	writeFile(t, filepath.Join(root, "tasks", "001-a.md"), "---\nid: \"1\"\n---\n")
	writeFile(t, filepath.Join(root, "other", "002-b.md"), "---\nid: \"2\"\n---\n")
	chdir(t, root)

	proj, err := discoverProject("other")
	if err != nil {
		t.Fatal(err)
	}
	if want := resolve(t, filepath.Join(root, "other")); proj.TaskDir != want {
		t.Errorf("task dir = %q, want %q", proj.TaskDir, want)
	}
	// The config still supplies the root the CLI is run from.
	if want := resolve(t, root); proj.Root != want {
		t.Errorf("root = %q, want %q", proj.Root, want)
	}
}

// -d can point into a project we are not standing in; its config is the one
// that names the phases and roots the `taskmd` calls.
func TestDiscoverProjectOverrideIntoAnotherProject(t *testing.T) {
	elsewhere := t.TempDir()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, configName), "dir: ./tasks\nphases:\n  - id: core\n    name: \"Core\"\n")
	writeFile(t, filepath.Join(root, "tasks", "001-a.md"), "---\nid: \"1\"\n---\n")
	chdir(t, elsewhere)

	proj, err := discoverProject(filepath.Join(root, "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	if want := resolve(t, root); proj.Root != want {
		t.Errorf("root = %q, want %q", proj.Root, want)
	}
	if got := proj.phaseLabel("core"); got != "Core" {
		t.Errorf("phaseLabel(core) = %q, want Core", got)
	}
}

// Without a config, a `tasks/` beside us is the layout `taskmd init` creates.
func TestDiscoverProjectWithoutConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "tasks", "001-a.md"), "---\nid: \"1\"\n---\n")
	chdir(t, root)

	proj, err := discoverProject("")
	if err != nil {
		t.Fatal(err)
	}
	if want := resolve(t, filepath.Join(root, "tasks")); proj.TaskDir != want {
		t.Errorf("task dir = %q, want %q", proj.TaskDir, want)
	}
}

// A bare pile of task files is only scanned when -d asks for it: falling back
// to the working directory would walk whatever it happens to be, every reload.
func TestDiscoverProjectBareDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "001-a.md"), "---\nid: \"1\"\n---\n")
	chdir(t, root)

	if _, err := discoverProject(""); err == nil {
		t.Error("a directory that is not a taskmd project should be refused")
	}

	proj, err := discoverProject(".")
	if err != nil {
		t.Fatal(err)
	}
	if want := resolve(t, root); proj.TaskDir != want {
		t.Errorf("task dir = %q, want %q", proj.TaskDir, want)
	}
}

func TestDiscoverProjectMissingDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, configName), "dir: ./nowhere\n")
	chdir(t, root)

	if _, err := discoverProject(""); err == nil {
		t.Error("a config pointing at a missing directory should fail")
	}
}

// A config that fails to parse should not stop the TUI: the fallbacks cover
// everything it would have supplied.
func TestDiscoverProjectMalformedConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, configName), "dir: [unclosed\n")
	writeFile(t, filepath.Join(root, "tasks", "001-a.md"), "---\nid: \"1\"\n---\n")
	chdir(t, root)

	proj, err := discoverProject("")
	if err != nil {
		t.Fatal(err)
	}
	if want := resolve(t, filepath.Join(root, "tasks")); proj.TaskDir != want {
		t.Errorf("task dir = %q, want %q", proj.TaskDir, want)
	}
}

// resolve follows symlinks so comparisons hold on platforms where the temp
// directory is one (/tmp -> /private/tmp on darwin).
func resolve(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
