package clean

import (
	"strings"
	"testing"
)

// The fixture mirrors the real pane observed in w6:p1: a failed command, a
// first k session that fixed it (invocation row, send-text echo row, executed
// command row), output, a canceled k session (invocation row, blank, prompt),
// then the current invocation row as the tail (what a live read sees).
const fixture = `root@zqr:~/code/kao# git staatus
git: 'staatus' is not a git command. See 'git --help'.

The most similar command is
        status
root@zqr:~/code/kao# ./k
git status
root@zqr:~/code/kao# git status
On branch main
Your branch is up to date with 'origin/main'.

Changes to be committed:
  (use "git restore --staged <file>..." to unstage)
        renamed:    internal/analyze/analyze.go -> internal/recommend/recommend.go

Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
  (use "git restore <file>..." to discard changes in working directory)
        modified:   README.md
        modified:   internal/recommend/recommend.go
        modified:   internal/ui/select.go
        modified:   main.go

root@zqr:~/code/kao# ./k

root@zqr:~/code/kao#
root@zqr:~/code/kao# ./k`

func TestSplitFixture(t *testing.T) {
	got := Split(fixture, []string{"./k"})

	// The last real command before the current run is "git status"; its row
	// and full output must be the analysis target, with the previous k
	// invocation and prompt-only rows removed.
	if !strings.HasPrefix(got.Recent, "root@zqr:~/code/kao# git status\nOn branch main") {
		t.Fatalf("Recent must start at the last real command row:\n%q", got.Recent)
	}
	for _, banned := range []string{"# ./k", "\nroot@zqr:~/code/kao#\n"} {
		if strings.Contains(got.Recent, banned) {
			t.Errorf("Recent still contains kao artifact %q:\n%s", banned, got.Recent)
		}
	}
	if !strings.Contains(got.Recent, "modified:   main.go") {
		t.Errorf("Recent must contain the git status output tail:\n%s", got.Recent)
	}

	// Context keeps the earlier failed command and its error output, with the
	// k session artifacts scrubbed: invocation rows, the bare prefill echo
	// row, and prompt-only rows.
	if !strings.Contains(got.Context, "git staatus") {
		t.Errorf("Context must keep the earlier failed command:\n%s", got.Context)
	}
	if !strings.Contains(got.Context, "'staatus' is not a git command") {
		t.Errorf("Context must keep the error output:\n%s", got.Context)
	}
	if strings.Contains(got.Context, "\ngit status\n") {
		t.Errorf("Context still contains the bare prefill echo row:\n%s", got.Context)
	}
	if strings.Contains(got.Context, "root@zqr:~/code/kao# git status") {
		t.Errorf("executed command row belongs in Recent, not Context:\n%s", got.Context)
	}
	if strings.Count(got.Context, "# ./k") != 0 {
		t.Errorf("Context still contains k invocation rows:\n%s", got.Context)
	}
}

func TestSplitKeepsRecentAndContextApart(t *testing.T) {
	got := Split(fixture, []string{"./k"})
	if got.Recent == "" || got.Context == "" {
		t.Fatalf("both regions expected: recent=%q context=%q", got.Recent, got.Context)
	}
	// Rows must not appear in both regions.
	recentLines := set(got.Recent)
	for _, l := range strings.Split(got.Context, "\n") {
		if l != "" && recentLines[l] {
			t.Errorf("row duplicated across regions: %q", l)
		}
	}
}

func TestSplitAliasInvocation(t *testing.T) {
	// The typed row may not equal argv's full form (e.g. the binary is named
	// "k" but invoked as an alias "kao"); ps1 must still be derived.
	s := "usr@h:~$ git log -1\ncommit abc\nusr@h:~$ kao"
	got := Split(s, []string{"k"})
	if !strings.Contains(got.Recent, "git log -1") {
		t.Fatalf("expected git log anchored in Recent, got:\n%q", got.Recent)
	}
	if strings.Contains(got.Recent, "# kao") || strings.Contains(got.Recent, "$ kao") {
		t.Errorf("invocation row leaked into Recent: %q", got.Recent)
	}
}

func TestSplitFlaggedInvocation(t *testing.T) {
	s := "usr@h:~$ true\nusr@h:~$ k --lines 300"
	got := Split(s, []string{"k", "--lines", "300"})
	if !strings.Contains(got.Recent, "true") {
		t.Fatalf("flag form failed to anchor: %q", got.Recent)
	}
}

func TestSplitNoAnchorFreshPane(t *testing.T) {
	// Fresh pane: only a prompt row and our invocation. Nothing to recommend.
	s := "usr@h:~$\nusr@h:~$ k"
	got := Split(s, []string{"k"})
	if got.Recent != "" {
		t.Fatalf("fresh pane should yield empty Recent, got %q", got.Recent)
	}
}

func TestSplitTailWordCollision(t *testing.T) {
	// A real command ending in token "k" before our invocation must not be
	// mistaken for a k invocation row when deriving the anchor.
	s := "usr@h:~$ ls /tmp/foo/k\nfile\nusr@h:~$ ./k"
	got := Split(s, []string{"./k"})
	if !strings.Contains(got.Recent, "ls /tmp/foo/k") {
		t.Fatalf("real command ending in k must stay in Recent:\n%q", got.Recent)
	}
}

func TestSplitScrubsUIResidue(t *testing.T) {
	s := `usr@h:~$ make
make: *** No targets.  exit 2
usr@h:~$ ./k
选择要填入的命令 (回车填充; 再次回车执行; Ctrl+C 取消)
  ▸ make -j4  build everything
  ▸ make test  run tests
usr@h:~$ ./k`
	got := Split(s, []string{"./k"})
	if strings.Contains(got.Context, "选择要填入的命令") || strings.Contains(got.Context, "▸ ") {
		t.Fatalf("UI residue not scrubbed from Context:\n%s", got.Context)
	}
	if !strings.Contains(got.Recent, "make") {
		t.Fatalf("real command must remain in Recent:\n%s", got.Recent)
	}
}

func TestSplitEmpty(t *testing.T) {
	if got := Split("", []string{"k"}); got.Recent != "" || got.Context != "" {
		t.Fatalf("empty snapshot must yield empty Result, got %+v", got)
	}
}

func TestSplitPreservesInteriorBlanks(t *testing.T) {
	// git-style output with interior blank lines must keep them.
	s := "usr@h:~$ git status\nOn branch main\n\nChanges:\nusr@h:~$ k"
	got := Split(s, []string{"k"})
	if !strings.Contains(got.Recent, "On branch main\n\nChanges:") {
		t.Fatalf("interior blank lines lost:\n%q", got.Recent)
	}
}

func set(s string) map[string]bool {
	m := map[string]bool{}
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			m[l] = true
		}
	}
	return m
}
