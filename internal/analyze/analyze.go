package analyze

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kiry163/easyllm"
	"github.com/kiry163/kao/internal/config"
)

// Suggestion is one proposed fix/follow-up command.
type Suggestion struct {
	Cmd  string `json:"cmd"`  // 修复/后续命令
	Desc string `json:"desc"` // 命令作用说明
}

// AnalyzeArgs is the model-facing structured tool payload.
type AnalyzeArgs struct {
	Command     string       `json:"command" tool:"required,desc=识别出的上一条真实用户命令(不含 kao/k 调用)"`
	Success     bool         `json:"success" tool:"required,desc=该命令是否执行成功"`
	Confidence  string       `json:"confidence" tool:"required,enum=high|low,desc=对 success 判断的把握"`
	Reason      string       `json:"reason" tool:"required,desc=简短中文诊断"`
	Suggestions []Suggestion `json:"suggestions" tool:"desc=0-3 条建议;单行完整可执行命令;禁止占位符"`
}

type analyzeTool struct {
	mu  sync.Mutex
	got *AnalyzeArgs
}

func (a *analyzeTool) Name() string { return "analyze_terminal" }

func (a *analyzeTool) Description() string {
	return "分析终端滚动缓冲,识别上一条真实用户命令,判断成败并给出修复建议"
}

func (a *analyzeTool) Run(_ context.Context, _ easyllm.ToolCallContext, args AnalyzeArgs) (easyllm.ToolResult, error) {
	a.mu.Lock()
	a.got = &args
	a.mu.Unlock()
	return easyllm.ToolResult{Message: "ok"}, nil
}

// Result is the parsed, validated analysis outcome.
type Result struct {
	Command     string
	Success     bool
	Confidence  string
	Reason      string
	Suggestions []Suggestion
}

// Analyze asks the model to identify the last real command in the snapshot and
// propose fixes. workdir/shell/os header the input so the model can ground its
// analysis.
func Analyze(ctx context.Context, cfg config.Config, snapshot, workdir, shell string) (*Result, error) {
	tool := &analyzeTool{}
	modelTool, err := easyllm.NewTool[AnalyzeArgs](tool, easyllm.WithUnknownArguments(easyllm.UnknownArgumentsWarn))
	if err != nil {
		return nil, fmt.Errorf("构建分析工具失败: %w", err)
	}

	client, err := easyllm.NewClient(easyllm.Config{
		Provider:       cfg.Provider,
		APIKey:         cfg.APIKey,
		BaseURL:        cfg.BaseURL,
		Model:          cfg.Model,
		EnableThinking: &cfg.Thinking,
		Timeout:        120 * time.Second,
		MaxRetries:     1,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化模型客户端失败: %w", err)
	}

	opts := []easyllm.EngineOption{
		easyllm.WithInstructions(systemPrompt()),
		easyllm.WithTools(modelTool),
		easyllm.WithParallelToolExecution(false),
		easyllm.WithMaxModelCalls(3),
		easyllm.WithStopOnToolSuccess(modelTool),
	}
	if !cfg.Thinking {
		opts = append(opts, easyllm.WithToolChoice(easyllm.NamedToolChoice(tool.Name())))
	}
	engine := easyllm.NewEngine(client, opts...)

	input := fmt.Sprintf("工作目录: %s\nShell: %s\nOS: %s\n\n以下是 pane 滚动缓冲(末尾为用户刚输入的 kao):\n%s",
		workdir, shell, runtime.GOOS, snapshot)

	result, err := engine.Run(ctx, easyllm.RunRequest{Input: input})
	if err != nil {
		return nil, fmt.Errorf("模型分析失败: %w", err)
	}

	tool.mu.Lock()
	got := tool.got
	tool.mu.Unlock()
	if result.StopReason != easyllm.StopReasonToolSucceeded || got == nil {
		return nil, fmt.Errorf("模型未返回结构化结果;如开启了 thinking 可关闭后重试")
	}
	return &Result{
		Command:     got.Command,
		Success:     got.Success,
		Confidence:  got.Confidence,
		Reason:      got.Reason,
		Suggestions: got.Suggestions,
	}, nil
}

func systemPrompt() string {
	return strings.Join([]string{
		"你是终端诊断助手。输入是用户 pane 的滚动缓冲末尾,最后一行是用户在提示符后输入的 kao/k(本工具调用)。",
		"任务:定位 kao/k 之前最近的一条真实用户命令及其输出区域,判断成败,输出修复建议。",
		"规则:",
		"(1) 忽略最后一行 kao/k 调用及其提示符;只分析它之前最近的真实命令和该命令的输出。",
		"(2) 依据输出证据(报错文本、堆栈、usage 提示、命令回显)判断 success;命令仍在运行或输出含糊时 confidence 用 low。",
		"(3) suggestions 最多 3 条;每条必须是完整、可直接执行且无需再编辑的单行命令;禁止占位符(<...>、path/to、your_* 等)。",
		"(4) desc 用简短中文说明命令作用/为何能修复。",
		"(5) 若确实没有真正有用的修复,返回空的 suggestions 列表并在 reason 中说明。",
		"输出必须调用 analyze_terminal 工具,不要用正文回复。",
	}, "\n")
}
