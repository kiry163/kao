package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/kiry163/kao/internal/clean"
	"github.com/kiry163/kao/internal/config"
	"github.com/kiry163/kao/internal/herdr"
	"github.com/kiry163/kao/internal/recommend"
	"github.com/kiry163/kao/internal/ui"
	"github.com/kiry163/kao/internal/workspace"
)

// Build-time version info, injected via
//
//	go build -ldflags "-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE"
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// controlChars guards pane text injection: a recommended command carrying raw
// control characters could drive the terminal rather than sit in the input
// line, so it is refused.
var controlChars = regexp.MustCompile(`[\x00-\x1f\x7f]`)

func versionString() string {
	parts := []string{"kao", version}
	if commit != "" {
		parts = append(parts, "commit "+commit)
	}
	if date != "" {
		parts = append(parts, date)
	}
	return strings.Join(parts, " ")
}

func run() error {
	// k has no option flags: every argument is goal text. Only -v/-h meta
	// invocations are intercepted; goals starting with "-" pass through.
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "-h", "-help", "--help":
			usage()
			return nil
		case "-v", "--version":
			fmt.Println(versionString())
			return nil
		}
	}
	query := strings.TrimSpace(strings.Join(args, " "))

	// herdr gate: must run inside a herdr pane.
	client, err := herdr.New()
	if err != nil {
		return err
	}
	paneID, _ := client.EnvPaneID()
	if !client.EnvOK() || paneID == "" {
		return fmt.Errorf("kao 只能在 herdr pane 内运行 (缺少 HERDR_ENV/HERDR_PANE_ID)")
	}

	// Config. First run creates the default file.
	path := config.DefaultPath()
	if _, err := config.EnsureDefault(path); err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	// Read pane snapshot.
	snapshot, err := client.ReadPane(paneID, cfg.SnapshotLines)
	if err != nil {
		return err
	}
	if snapshot == "" {
		return fmt.Errorf("pane 滚动缓冲为空,无内容可分析")
	}

	workdir, _ := os.Getwd()
	shell := os.Getenv("SHELL")

	// Clean kao traces from the snapshot and split it: Recent is the last real
	// command plus its output, Context is older scrollback for background.
	part := clean.Split(snapshot, os.Args)

	// Query mode grounds the goal with a read-only workspace snapshot (tree,
	// else ls); context mode relies on the buffer alone.
	var ws string
	if query != "" {
		ws = workspace.Snapshot(workdir)
	}

	ctx := context.Background()
	cmds, err := recommend.Recommend(ctx, *cfg, recommend.Input{
		Query:     query,
		Recent:    part.Recent,
		Context:   part.Context,
		Workspace: ws,
		Workdir:   workdir,
		Shell:     shell,
	})
	if err != nil {
		return err
	}

	if len(cmds) == 0 {
		fmt.Fprintln(os.Stderr, "没有可推荐命令")
		return nil
	}

	sel, err := ui.Select(cmds)
	if err != nil {
		return err
	}
	if sel == nil {
		return nil // 用户取消
	}

	if controlChars.MatchString(sel.Command) {
		fmt.Fprintln(os.Stderr, "该推荐含控制字符,已跳过")
		return nil
	}
	cmd := strings.TrimSpace(sel.Command)

	// Prefill only, never execute: inject the chosen command into the pane's
	// input line for the user to edit/press Enter. The detached send waits for
	// the shell prompt to reappear so the text lands on a fresh line, not on
	// the still-foreground k. The echo it leaves is scrubbed on the next run.
	if err := client.SendText(paneID, cmd, part.Prompt); err != nil {
		return err
	}
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "用法: k [目标描述]\n\n")
	fmt.Fprintf(os.Stderr, "不带参数: 基于最近命令推荐(修复失败命令或给出下一步)。\n")
	fmt.Fprintf(os.Stderr, "带参数:   把参数当作目标,推荐达成该目标的命令,例如:\n")
	fmt.Fprintf(os.Stderr, "           k 把当前改动提交并推送\n\n")
	fmt.Fprintf(os.Stderr, "其他: k -v 打印版本; k -h 打印本帮助。\n")
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
