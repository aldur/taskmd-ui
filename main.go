// Command taskmd-ui is a full-screen browser for a taskmd project.
//
// taskmd ships a CLI and a web dashboard; upstream removed its own TUI in
// v0.3.0 as redundant with those two. This is the terminal view they left
// behind: a list you filter and re-sort a keypress at a time, next to the
// detail of whatever is under the cursor.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// version is the release this source is. Kept here rather than stamped in by
// the build so that a packaging pin claiming one version while the source says
// another is a test failure, not a silent lie.
const version = "0.1.0"

func main() {
	var (
		taskDir     string
		showVersion bool
	)
	flag.StringVar(&taskDir, "d", "", "task directory to scan (default: `dir` from .taskmd.yaml)")
	flag.StringVar(&taskDir, "task-dir", "", "same as -d")
	flag.BoolVar(&showVersion, "version", false, "print the version and exit")
	flag.Usage = usage
	flag.Parse()

	if showVersion {
		fmt.Printf("taskmd-ui version %s\n", version)
		return
	}

	proj, err := discoverProject(taskDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "taskmd-ui: %v\n", err)
		os.Exit(1)
	}

	program := tea.NewProgram(newModel(proj), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "taskmd-ui: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(flag.CommandLine.Output(), `Usage: taskmd-ui [-d DIR]

Browse a taskmd project: filter by status, priority, tag, phase, owner, group
and type, sort by any column, and move tasks along the workflow. Press ? inside
for the full keymap.

The / prompt takes an expression, live as you type:

  status!=completed        not this value (also -status:completed)
  status:open              open, active, done or resolved as a group
  priority>=high           at least this urgent (effort orders too)
  tag:a11y,ux              either value
  owner:none               field is unset (owner:any for the opposite)
  created<2026-06-01       dates compare as dates
  "record format"          free text, quoted to keep the space

Terms are ANDed. Fields: status priority effort type owner phase group tag dep
touches parent id title file body created.

The task directory is discovered the way taskmd discovers it — the `+"`dir`"+`
key of the nearest .taskmd.yaml above the working directory — unless -d says
otherwise.

Options:
  -d, --task-dir DIR   task directory to scan
      --version        print the version and exit
  -h, --help           show this help
`)
}
