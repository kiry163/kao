// Package workspace produces a small read-only snapshot of the current
// working directory for the model: a two-level tree when the tree command is
// available, otherwise a plain ls. It never modifies the workspace.
package workspace

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

const maxLines = 200

// ignoreDirs excludes common generated/dependency directories from the tree
// so the snapshot stays small and focused on authored content.
const ignoreDirs = ".git|node_modules|target|dist|build|vendor|__pycache__|.venv|venv"

// Snapshot returns a bounded textual snapshot of dir. tree is preferred; if it
// is not installed, ls -la of the top level is used. Any failure (missing dir,
// unreadable, timeout) yields an empty string: the workspace view is a
// best-effort aid, never a reason to fail the run.
func Snapshot(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var out []byte
	var err error
	if _, lookErr := exec.LookPath("tree"); lookErr == nil {
		out, err = run(ctx, "tree", "-a", "-I", ignoreDirs, "-L", "2", "--dirsfirst", "--noreport", dir)
	} else {
		out, err = run(ctx, "ls", "-la", dir)
	}
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], "(输出已截断)")
	}
	return strings.Join(lines, "\n")
}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}
