package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Writes go through the `taskmd` binary rather than editing frontmatter here.
// It owns the rules — which transitions are legal, what else a status change
// touches (`completed_at`), how the file is re-serialised — and duplicating
// any of that would mean two implementations to keep in step. Reads stay local
// (see task.go); only mutations pay for a subprocess.

// setStatusCmd runs `taskmd set` against the selected task.
func setStatusCmd(proj Project, task Task, status string) tea.Cmd {
	return func() tea.Msg {
		if task.Status == status {
			return actionMsg{notice: fmt.Sprintf("%s is already %s", task.ID, status)}
		}
		cmd := exec.Command("taskmd", "--task-dir", proj.TaskDir, "set", "--task-id", task.ID, "--status", status)
		cmd.Dir = proj.Root
		out, err := cmd.CombinedOutput()
		if err != nil {
			return actionMsg{err: fmt.Errorf("taskmd set: %s", firstLine(string(out), err))}
		}
		return actionMsg{notice: fmt.Sprintf("%s  %s → %s", task.ID, orDash(task.Status), status)}
	}
}

// editCmd hands the terminal to $EDITOR and takes it back when the editor
// exits; the reload that follows picks up whatever was saved.
func editCmd(task Task) tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	// $EDITOR carrying flags ("code -w", "emacsclient -nw") is common enough
	// to split on, and a bare path with spaces is not.
	parts := strings.Fields(editor)
	args := append(parts[1:], task.Path)

	return tea.ExecProcess(exec.Command(parts[0], args...), func(err error) tea.Msg {
		if err != nil {
			return actionMsg{err: fmt.Errorf("%s: %w", parts[0], err)}
		}
		return actionMsg{}
	})
}

// firstLine picks the most useful line out of a failed command's output: its
// last non-empty one, which is where cobra puts the error, falling back to the
// exit status when it printed nothing.
func firstLine(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return err.Error()
}
