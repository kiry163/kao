package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kiry163/kao/internal/analyze"
	"github.com/kiry163/kao/internal/config"
	"github.com/kiry163/kao/internal/herdr"
	"github.com/kiry163/kao/internal/ui"
)

var (
	debugMode bool
	linesFlag int
	printFlag bool
	autoExec  bool
	execSet   bool
)

func debugLog(format string, args ...interface{}) {
	if debugMode {
		fmt.Fprintf(os.Stderr, "[kao] "+format+"\n", args...)
	}
}

var controlChars = regexp.MustCompile(`[\x00-\x1f\x7f]`)

func run() error {
	flag.BoolVar(&debugMode, "d", false, "开启调试日志")
	flag.BoolVar(&debugMode, "debug", false, "开启调试日志")
	flag.IntVar(&linesFlag, "lines", 0, "读取 pane 行数 (0 = 使用配置 snapshot_lines)")
	flag.BoolVar(&printFlag, "print", false, "纯净模式: 选中命令打印到 stdout,列表走 stderr(供 zsh print -z 函数)")
	flag.BoolVar(&autoExec, "auto-execute", false, "覆盖配置文件: 选中后直接执行命令")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "auto-execute" {
			execSet = true
		}
	})

	// herdr gate: must run inside a herdr pane.
	client, err := herdr.New()
	if err != nil {
		return err
	}
	paneID, _ := client.EnvPaneID()
	if !client.EnvOK() || paneID == "" {
		return fmt.Errorf("kao 只能在 herdr pane 内运行 (缺少 HERDR_ENV/HERDR_PANE_ID)")
	}
	debugLog("pane id: %s", paneID)

	// Config. First run creates the default file; no guided-prompt message.
	path := config.DefaultPath()
	if _, err := config.EnsureDefault(path); err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	// Read pane snapshot.
	lines := cfg.SnapshotLines
	if linesFlag > 0 {
		lines = linesFlag
	}
	debugLog("读取 %d 行 pane 输出", lines)
	snapshot, err := client.ReadPane(paneID, lines)
	if err != nil {
		return err
	}
	if snapshot == "" {
		return fmt.Errorf("pane 滚动缓冲为空,无内容可分析")
	}

	workdir, _ := os.Getwd()
	shell := os.Getenv("SHELL")

	ctx := context.Background()
	res, err := analyze.Analyze(ctx, *cfg, snapshot, workdir, shell)
	if err != nil {
		return err
	}

	if len(res.Suggestions) == 0 {
		fmt.Fprintln(os.Stderr, "没有可执行建议")
		return nil
	}

	sel, err := ui.Select(res.Suggestions)
	if err != nil {
		return err
	}
	if sel == nil {
		debugLog("用户取消选择")
		return nil
	}

	if controlChars.MatchString(sel.Cmd) {
		fmt.Fprintln(os.Stderr, "该建议含控制字符,未执行")
		return nil
	}
	cmd := strings.TrimSpace(sel.Cmd)

	// --print: selected command to stdout (clean), list already on stderr.
	if printFlag {
		fmt.Println(cmd)
		return nil
	}

	// Explicit -auto-execute wins; otherwise config auto_execute, default false
	// (prefill).
	execute := cfg.AutoExecute
	if execSet {
		execute = autoExec
	}

	if execute {
		debugLog("执行命令: %s", cmd)
		if err := client.RunPane(paneID, cmd); err != nil {
			return err
		}
		return nil
	}

	debugLog("填入命令: %s", cmd)
	if err := client.SendText(paneID, cmd); err != nil {
		return err
	}
	return nil
}

func initShell() error {
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行路径失败: %w", err)
	}
	shell := os.Getenv("SHELL")
	target := ""
	var block string
	switch filepath.Base(shell) {
	case "zsh":
		target = filepath.Join(os.Getenv("HOME"), ".zshrc")
		block = fmt.Sprintf(`
# kao: 选中后把命令填入当前输入缓冲区(不自动执行),避免 PTY 回显
k() {
  local c
  c=$(%q --print)
  [[ -n "$c" ]] && print -z -- "$c"
}
`, bin)
	case "bash":
		target = filepath.Join(os.Getenv("HOME"), ".bashrc")
		block = fmt.Sprintf(`
# kao: bash 无 print -z,用 bind -x 绑定 Ctrl-X 触发填充
k() {
  local c
  c=$(%q --print)
  if [[ -n "$c" ]]; then READLINE_LINE="$c"; READLINE_POINT=${#c}; fi
}
bind -x '"\C-xk":k'
`, bin)
	default:
		return fmt.Errorf("不支持的 shell: %s (仅支持 zsh/bash)", shell)
	}

	marker := "# kao: k 函数"
	data, _ := os.ReadFile(target)
	// Marker-less existing k() would still be overwritten by appending; guard by
	// only appending when the marker (and thus our function) is not present.
	if strings.Contains(string(data), marker) {
		fmt.Printf("已在 %s 中检测到 kao k 函数,跳过。\n", target)
		return nil
	}
	f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开 %s 失败: %w", target, err)
	}
	defer f.Close()
	if _, err := f.WriteString(marker + block); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", target, err)
	}
	fmt.Printf("已把 kao k 函数写入 %s,请执行 source %s 或重启 shell 生效。\n", target, target)
	fmt.Println("提示: SHELL 为 " + shell + ",函数采用 " + filepath.Base(shell) + " 的填充机制。")
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "用法: k [flags] | k init\n\nflags:\n")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "\ninit: 把 k shell 函数写入 ~/.zshrc 或 ~/.bashrc,选中后命令填入缓冲区(无 PTY 回显)。")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := initShell(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		if err == flag.ErrHelp {
			usage()
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
