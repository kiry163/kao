package recommend

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

// Recommendation is one proposed command.
type Recommendation struct {
	Command string `json:"command"` // 建议执行的命令
	Desc    string `json:"desc"`    // 推荐原因:作用或修复点
}

// RecommendArgs is the model-facing structured tool payload. An empty Commands
// list is valid and means "nothing worth recommending".
type RecommendArgs struct {
	Commands []Recommendation `json:"commands" tool:"required,desc=推荐命令列表;可为空数组表示无可推荐"`
}

// Input is everything Recommend knows about the user's situation. Query mode
// (Query != "") recommends commands that achieve the stated goal; context mode
// recommends a fix or next step for the last real command.
type Input struct {
	Query     string // 自然语言目标("k 提交这些改动");空 = 上下文模式
	Recent    string // 最近一条真实命令 + 输出(kao 痕迹已清除)
	Context   string // 更早历史(kao 痕迹已清除,仅背景)
	Workspace string // 查询模式的工作区快照(tree/ls),可为空
	Workdir   string
	Shell     string
}

type recommendTool struct {
	mu  sync.Mutex
	got *RecommendArgs
}

func (t *recommendTool) Name() string { return "recommend_commands" }

func (t *recommendTool) Description() string {
	return "根据用户目标与终端上下文,推荐用户应执行的命令列表(修正失败命令、给出下一步,或达成目标)"
}

func (t *recommendTool) Run(_ context.Context, _ easyllm.ToolCallContext, args RecommendArgs) (easyllm.ToolResult, error) {
	t.mu.Lock()
	t.got = &args
	t.mu.Unlock()
	return easyllm.ToolResult{Message: "ok"}, nil
}

// Recommend asks the model for the commands the user should run next.
func Recommend(ctx context.Context, cfg config.Config, in Input) ([]Recommendation, error) {
	tool := &recommendTool{}
	modelTool, err := easyllm.NewTool[RecommendArgs](tool, easyllm.WithUnknownArguments(easyllm.UnknownArgumentsWarn))
	if err != nil {
		return nil, fmt.Errorf("构建推荐工具失败: %w", err)
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
		easyllm.WithInstructions(systemPrompt(in.Query != "")),
		easyllm.WithTools(modelTool),
		easyllm.WithParallelToolExecution(false),
		easyllm.WithMaxModelCalls(3),
		easyllm.WithStopOnToolSuccess(modelTool),
	}
	if !cfg.Thinking {
		opts = append(opts, easyllm.WithToolChoice(easyllm.NamedToolChoice(tool.Name())))
	}
	engine := easyllm.NewEngine(client, opts...)

	result, err := engine.Run(ctx, easyllm.RunRequest{Input: buildInput(in)})
	if err != nil {
		return nil, fmt.Errorf("模型推荐失败: %w", err)
	}

	tool.mu.Lock()
	got := tool.got
	tool.mu.Unlock()
	if result.StopReason != easyllm.StopReasonToolSucceeded || got == nil {
		return nil, fmt.Errorf("模型未返回结构化结果;如开启了 thinking 可关闭后重试")
	}
	return got.Commands, nil
}

// buildInput assembles the model prompt body. Common header first; query mode
// adds the goal and the workspace snapshot; both modes carry the two scrubbed
// terminal regions, labeled so the model never guesses which command is the
// analysis target.
func buildInput(in Input) string {
	var b strings.Builder
	fmt.Fprintf(&b, "工作目录: %s\nShell: %s\nOS: %s\n", in.Workdir, in.Shell, runtime.GOOS)

	if in.Query != "" {
		b.WriteString("\n用户目标: ")
		b.WriteString(in.Query)
		b.WriteString("\n(以下是参考信息:当前工作区与最近终端历史。推荐必须服务于达成该目标。)")
		if in.Workspace != "" {
			b.WriteString("\n\n—— 当前工作区内容(tree/ls,只读快照) ——\n")
			b.WriteString(in.Workspace)
		}
	}
	if in.Recent != "" {
		if in.Query != "" {
			b.WriteString("\n\n—— 最近命令区(参考:上一条真实命令及其输出) ——\n")
		} else {
			b.WriteString("\n—— 最近命令区(分析对象:最后一条真实命令及其输出) ——\n")
		}
		b.WriteString(in.Recent)
	}
	if in.Context != "" {
		b.WriteString("\n\n—— 更早上下文区(仅供背景参考;kao/k 自身痕迹已清除) ——\n")
		b.WriteString(in.Context)
	}
	return b.String()
}

// systemPrompt returns the mode-specific instructions. queryMode asks for
// commands that achieve the stated goal and allows placeholders for values
// only the user knows — they are filled into the command line for the user to
// edit before pressing Enter. Context mode asks for a fix or next step and
// bans placeholders (the fix must be ready to run).
func systemPrompt(queryMode bool) string {
	if queryMode {
		return strings.Join([]string{
			"你是终端命令推荐助手。用户给出了目标,但没有说具体命令。",
			"输入含:用户目标、当前工作区内容(tree/ls 快照)、最近命令区、更早上下文区。RECENT/CONTEXT/工作区都只是参考,不是分析对象。",
			"任务:推荐达成用户目标需要执行的命令。",
			"规则:",
			"(1) 推荐必须直接服务于目标;可以一次推荐多条命令(如 git add 后再 git commit),按执行顺序排列。",
			"(2) 命令中的目录/文件路径要基于工作区快照与工作目录写实,不要编造不存在的路径。",
			"(3) 只有用户本人知道的值(提交信息、分支名、密钥、目标文件名等)允许用占位符表示:格式 <英文大写,如 COMMIT_MESSAGE>;其余部分必须完整可直接执行。用户选中后会在命令行里编辑占位符再回车。",
			"(4) 不要为了用上缓冲而修复旧命令;目标与旧命令无关时忽略它们。",
			"(5) 若目标在当前上下文下无法达成或证据不足,返回空列表并保持克制。",
			"(6) 最多 3 条;desc 用简短中文说明这条命令为什么能达成目标。",
			"输出必须调用 recommend_commands 工具返回 JSON,不要用正文回复。",
		}, "\n")
	}
	return strings.Join([]string{
		"你是终端命令推荐助手。输入分两段:RECENT 区是最近一条真实命令及其输出(分析对象),CONTEXT 区是更早的历史(仅背景参考)。kao/k 自身的调用行与 UI 输出已被程序清除,若历史区仍有零星 k 痕迹,一律忽略。",
		"任务:推荐用户接下来最该执行的一条或几条命令。",
		"规则:",
		"(1) RECENT 区首行即最后一条真实命令:若它明显失败(报错文本、堆栈、usage 提示等),第一条推荐应为修正后的完整可执行命令,后续可按需补充动作;若它成功,推荐合理的下一步。",
		"(2) 不要对 CONTEXT 区里的旧命令做修复推荐。",
		"(3) 若 RECENT 区为空或上下文不足以判断用户意图,返回空列表,不要硬凑。",
		"(4) 输出证据含糊(如命令仍在运行)时保守处理:证据不足宁可少推或不推。",
		"(5) 最多 3 条;每条 command 必须是完整、可直接执行且无需再编辑的单行命令;禁止占位符(<...>、path/to、your_* 等)。",
		"(6) desc 用简短中文说明推荐原因(修正了什么,或这条命令做什么、为什么现在适合执行)。",
		"输出必须调用 recommend_commands 工具返回 JSON,不要用正文回复。",
	}, "\n")
}
