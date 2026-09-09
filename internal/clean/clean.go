// Package clean removes kao/k's own traces from a pane snapshot before it is
// sent to the model and splits the remainder into a recent-command region (the
// analysis target) and older context.
//
// The pane text strips ANSI, so no prompt markers survive. k reads the pane at
// startup before its UI is drawn, so the snapshot's last row is always the row
// the user just typed: stripping our own invocation suffix from it yields the
// exact prompt string, which anchors the scan for the last real command row.
// No prompt configuration is needed.
package clean

import (
	"regexp"
	"strings"
)

// Result is the cleaned snapshot split into regions.
type Result struct {
	// Prompt is the derived session prompt with trailing separator trimmed
	// (e.g. "root@zqr:~/code/kao#"); empty when unvalidated. It lets the
	// caller wait for the shell to return to the prompt before injecting text.
	Prompt string
	// Recent is the last real command row plus everything that followed it,
	// with kao artifacts removed. It is the analysis target.
	Recent string
	// Context is the older scrollback kept only as background, scrubbed of
	// kao/k invocation rows and UI artifacts.
	Context string
}

// uiSignatures are rows that can only be kao UI output or messages.
var uiSignatures = []string{
	"选择要填入的命令", // promptui label
	"没有可推荐命令",
	"该推荐含控制字符",
}

// Split separates a pane snapshot into Recent and Context. argv is the current
// invocation (os.Args): its full form and base names are used to strip the
// invocation suffix from the tail row.
func Split(snapshot string, argv []string) Result {
	rows := strings.Split(strings.TrimSpace(snapshot), "\n")
	if len(rows) == 0 || (len(rows) == 1 && strings.TrimSpace(rows[0]) == "") {
		return Result{}
	}

	tail := strings.TrimSpace(rows[len(rows)-1])
	body := rows[:len(rows)-1] // the tail is our own invocation row

	ps1, ok := derivePS1(tail, argv, body)
	if !ok {
		// Cannot derive a validated prompt: scrub what we can and hand the
		// model a single region rather than guessing boundaries.
		scrubbed := scrub(body, "")
		return Result{Recent: strings.Join(scrubbed, "\n")}
	}
	prompt := strings.TrimRight(ps1, " \t")

	// Scrub the whole body first (echo-row removal needs to look past the
	// region boundary), then split at the last remaining real command row.
	scrubbed := scrub(body, ps1)
	anchor := lastCommandRow(scrubbed, ps1)
	if anchor < 0 {
		return Result{Prompt: prompt, Recent: strings.Join(scrubbed, "\n")}
	}
	return Result{
		Prompt:  prompt,
		Recent:  strings.Join(scrubbed[anchor:], "\n"),
		Context: strings.Join(scrubbed[:anchor], "\n"),
	}
}

// derivePS1 strips a known invocation spelling from the tail row
// ("<PS1><invocation>"). The candidate is accepted only if its prompt core
// prefixes an earlier row — a real prompt does; a command that merely ended in
// the token "k" does not.
func derivePS1(tail string, argv []string, earlier []string) (string, bool) {
	for _, c := range invocationCandidates(argv) {
		if !strings.HasSuffix(tail, c) {
			continue
		}
		ps1 := strings.TrimSuffix(tail, c)
		if ps1 == "" || strings.ContainsRune(ps1, '\n') {
			continue
		}
		core := strings.TrimRight(ps1, " \t")
		if !prefixSeen(core, earlier) {
			continue
		}
		return ps1, true
	}
	return "", false
}

// prefixSeen reports whether core prefixes any earlier (non-tail) row.
func prefixSeen(core string, earlier []string) bool {
	for _, r := range earlier {
		if strings.HasPrefix(r, core) {
			return true
		}
	}
	return false
}

// invocationCandidates lists spellings the tail row might end with: the full
// argv, its base name, then known aliases, longest first.
func invocationCandidates(argv []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(argv) > 0 {
		add(strings.Join(argv, " ")) // 全形: 如 "k 打包这个目录"
		add(argv[0])
		add(baseName(argv[0]))
	}
	add("./k")
	add("./kao")
	add("k")
	add("kao")
	// Longest first, so a flagged form is tried before its bare base.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if len(out[j]) > len(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// commandOf returns the trimmed command text of a row that starts with the
// session prompt ps1, and whether the row is a command row at all. ps1 may
// carry a trailing separator space that captured prompt-only rows lack, so the
// comparison uses its core.
func commandOf(row, ps1 string) (cmd string, isCmd bool) {
	core := strings.TrimRight(ps1, " \t")
	if !strings.HasPrefix(row, core) {
		return "", false
	}
	rest := strings.TrimSpace(row[len(core):])
	if rest == "" {
		return "", true // prompt-only row (empty command)
	}
	return rest, true
}

// lastCommandRow returns the index of the last real command row: scrub has
// already removed invocation and prompt-only rows, so any remaining command
// row here is a real one.
func lastCommandRow(rows []string, ps1 string) int {
	for i := len(rows) - 1; i >= 0; i-- {
		cmd, isCmd := commandOf(rows[i], ps1)
		if isCmd && cmd != "" {
			return i
		}
	}
	return -1
}

// invocationRE matches a kao/k command row: the invocation word, optionally
// "./"-prefixed, followed by any "-flag value" tokens. It stays generic so
// rows typed with older or future flags are scrubbed too.
var invocationRE = regexp.MustCompile(`^(\./)?(k|kao)(\s+-{1,2}[a-zA-Z][a-zA-Z-]*(\s+[^\s-]\S*)?)*$`)

func isInvocation(cmd string) bool {
	return invocationRE.MatchString(cmd)
}

// scrub removes kao artifacts from rows: invocation rows, prompt-only rows,
// kao UI rows, and prefill echo rows (see isEchoRow). Real output blank lines
// are kept; only leading/trailing blanks and doubled blanks left by removed
// rows collapse. With ps1 == "" only UI signatures are removed.
func scrub(rows []string, ps1 string) []string {
	out := make([]string, 0, len(rows))
	for i, r := range rows {
		if ps1 != "" {
			if cmd, isCmd := commandOf(r, ps1); isCmd && (cmd == "" || isInvocation(cmd)) {
				continue
			}
		}
		trimmed := strings.TrimSpace(r)
		if trimmed == "" {
			out = append(out, "")
			continue
		}
		if isUIRow(trimmed) || isEchoRow(rows, i, ps1) {
			continue
		}
		out = append(out, r)
	}
	return trimBlanks(out)
}

// isEchoRow reports whether the bare row at i is a prefill echo: a send-text
// injection echo lands between its k invocation row and the executed command
// row, and equals that command's text. Requiring both neighbors keeps a bare
// output line that merely equals the next command text.
func isEchoRow(rows []string, i int, ps1 string) bool {
	if ps1 == "" || strings.TrimSpace(rows[i]) == "" {
		return false
	}
	if _, isCmd := commandOf(rows[i], ps1); isCmd {
		return false // a prompt-bearing row is never an echo
	}
	prevInvocation := false
	for j := i - 1; j >= 0; j-- {
		pr := rows[j]
		if strings.TrimSpace(pr) == "" {
			continue
		}
		cmd, isCmd := commandOf(pr, ps1)
		prevInvocation = isCmd && cmd != "" && isInvocation(cmd)
		break
	}
	if !prevInvocation {
		return false
	}
	for j := i + 1; j < len(rows); j++ {
		nr := rows[j]
		if strings.TrimSpace(nr) == "" {
			continue
		}
		cmd, isCmd := commandOf(nr, ps1)
		return isCmd && cmd == strings.TrimSpace(rows[i])
	}
	return false
}

// isUIRow reports whether a trimmed row is kao UI output or a kao message.
func isUIRow(trimmed string) bool {
	// promptui items are "▸ cmd  desc"; the arrow only matches when it leads
	// the row, which real output rarely has.
	if strings.HasPrefix(trimmed, "▸ ") || strings.HasPrefix(trimmed, "❯ ") {
		return true
	}
	for _, s := range uiSignatures {
		if strings.HasPrefix(trimmed, s) {
			return true
		}
	}
	return false
}

// trimBlanks drops leading/trailing blank rows and collapses interior runs of
// two or more blank rows into one.
func trimBlanks(rows []string) []string {
	out := make([]string, 0, len(rows))
	blankRun := 0
	for _, r := range rows {
		if strings.TrimSpace(r) == "" {
			blankRun++
			if blankRun == 1 {
				out = append(out, "")
			}
			continue
		}
		blankRun = 0
		out = append(out, r)
	}
	if n := len(out); n > 0 && strings.TrimSpace(out[n-1]) == "" {
		out = out[:n-1]
	}
	return out
}
