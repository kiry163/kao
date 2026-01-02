package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"kao/internal/config"
	"runtime"
	"strings"

	"github.com/sashabaranov/go-openai"
)

type Client struct {
	openaiClient *openai.Client
	config       *config.Config
}

func NewClient(cfg *config.Config) *Client {
	c := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		c.BaseURL = cfg.BaseURL
	}
	return &Client{
		openaiClient: openai.NewClientWithConfig(c),
		config:       cfg,
	}
}

// Suggestion 结构化建议
type Suggestion struct {
	Cmd  string `json:"cmd"`  // 实际执行的命令
	Desc string `json:"desc"` // 命令作用说明
}

type IntentAnalysis struct {
	Type        string       `json:"type"`        // "typo", "logic", "unsafe"
	Suggestions []Suggestion `json:"suggestions"` // 修正建议
	Advice      string       `json:"advice"`      // 自然语言建议/指导
	Reason      string       `json:"reason"`      // 分析理由
	SafeToRun   bool         `json:"safeToRun"`   // 安全性
}

func (c *Client) AnalyzeIntent(ctx context.Context, cmd string) (*IntentAnalysis, error) {
	prompt := fmt.Sprintf(`你是一个智能终端助手。请分析用户输入的命令，判断其意图和安全性。

Current系统环境: %s/%s

命令: %s

请返回 JSON 格式结果，包含以下字段：
1. "type": "typo" (拼写错误), "logic" (逻辑/参数错误), "unsafe" (破坏性/副作用).
2. "advice": 针对该意图的自然语言建议或指导。
3. "suggestions": 建议列表，对象数组 [{"cmd": "...", "desc": "..."}].
   - 如果是 "typo"，列出 1-3 个修正命令。
   - "desc" 说明修正内容 (如 "修正拼写 branch")。
   - 严禁包含占位符。
4. "safeToRun": 布尔值。只读命令或安全重放命令为 true。
5. "reason": 简短中文说明。

示例: {
  "type": "typo", 
  "advice": "您似乎输错了命令，请检查拼写。",
  "suggestions": [{"cmd": "git branch", "desc": "修正拼写错误"}], 
  "safeToRun": false, 
  "reason": "拼写错误"
}
`, runtime.GOOS, runtime.GOARCH, cmd)

	resp, err := c.openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.config.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})

	if err != nil {
		return nil, err
	}

	content := cleanJSON(resp.Choices[0].Message.Content)

	var result IntentAnalysis
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return &IntentAnalysis{Type: "unsafe", SafeToRun: false, Reason: "AI返回格式异常"}, nil
	}

	return &result, nil
}

type ErrorAnalysis struct {
	Explanation string       `json:"explanation"` // 错误原因分析
	Advice      string       `json:"advice"`      // 自然语言建议
	Suggestions []Suggestion `json:"suggestions"` // 可执行命令列表
}

func (c *Client) AnalyzeError(ctx context.Context, cmd, output string) (*ErrorAnalysis, error) {
	// 截断逻辑
	const maxLen = 3000
	if len(output) > maxLen {
		output = output[:1500] + "\n... (中间内容已截断) ...\n" + output[len(output)-1500:]
	}

	prompt := fmt.Sprintf(`你是一个资深运维专家。请分析终端命令及其输出。

Current系统环境: %s/%s

执行命令: %s
执行输出:
%s

请分析并返回 JSON：
1. "explanation": 简明扼要的错误原因分析（或执行结果概括）。
2. "advice": 自然语言形式的修复或后续操作建议。
   - 如果需要用户手动操作（如修改代码、切换特定目录），请在这里详细说明。
   - 可以在这里解释为什么会推荐下面的命令。
3. "suggestions": 建议列表，对象数组 [{"cmd": "...", "desc": "..."}]。
   - 列出 1-3 个**完整、可直接执行**的修复或后续命令。
   - **严禁**包含占位符 (如 <path>)。如果无法给出确切命令，请留空，并在 "advice" 中说明。
   - "desc" 简短说明该命令的作用 (如 "安装 Rust", "初始化仓库")。

示例: {
  "explanation": "当前目录不是 Git 仓库。",
  "advice": "如果您想新建仓库，请使用 init；如果想使用现有仓库，请先 cd 到目标目录。",
  "suggestions": [{"cmd": "git init", "desc": "在当前目录初始化新仓库"}]
}
`, runtime.GOOS, runtime.GOARCH, cmd, output)

	resp, err := c.openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.config.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})

	if err != nil {
		return nil, err
	}

	content := cleanJSON(resp.Choices[0].Message.Content)

	var result ErrorAnalysis
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return &ErrorAnalysis{
			Explanation: content, // 降级
			Advice:      "",
			Suggestions: []Suggestion{},
		}, nil
	}

	return &result, nil
}

func cleanJSON(content string) string {
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}