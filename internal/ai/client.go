package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"kao/internal/config"
	"regexp"
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
1. "type": "typo", "logic", "unsafe".
2. "advice": 针对该意图的自然语言建议。请使用清晰的分点格式 (Markdown)，允许换行。
3. "suggestions": 建议列表 [{"cmd": "...", "desc": "..."}]。
   - 如果是 "typo"，列出 1-3 个修正命令。
   - "desc" 字段必须说明**这是什么修正**。
   - **严禁**包含需要用户修改的占位符 (如 <IP>, [file])，必须是完整可执行命令。
4. "safeToRun": 布尔值。
5. "reason": 简短中文说明。

示例: {
  "type": "typo", 
  "advice": "您似乎输错了命令，请检查拼写。",
  "suggestions": [{"cmd": "git branch", "desc": "修正拼写错误 'brnch' -> 'branch'"}], 
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

	result.Suggestions = sanitizeSuggestions(result.Suggestions)
	return &result, nil
}

type ErrorAnalysis struct {
	Explanation string       `json:"explanation"` // 错误原因分析
	Advice      string       `json:"advice"`      // 自然语言建议
	Suggestions []Suggestion `json:"suggestions"` // 可执行命令列表
}

func (c *Client) AnalyzeError(ctx context.Context, cmd, output string) (*ErrorAnalysis, error) {
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
2. "advice": 针对该情况的详细指导建议。
   - **格式要求**: 使用 Markdown 列表或分段，清晰易读。如果涉及无法自动执行的命令（如需要具体路径），请在这里列出并解释，**不要**放到 suggestions 里。
   - 可以在这里解释为什么会推荐下面的命令。
3. "suggestions": 建议列表 [{"cmd": "...", "desc": "..."}]。
   - 列出 1-3 个**完整、可直接执行**的修复或后续命令。
   - **绝对禁止**包含占位符 (如 <path>, /path/to, [file])。如果无法给出确切命令，请留空，只在 advice 中说明即可。
   - "desc" 简短说明该命令的作用。

示例: {
  "explanation": "当前目录不是 Git 仓库。",
  "advice": "您可以选择：\n1. 在当前目录初始化 (推荐)\n2. 切换到正确的仓库目录 (请手动执行 'cd path/to/repo')",
  "suggestions": [{"cmd": "git init", "desc": "在当前目录初始化"}]
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
			Explanation: content, 
			Advice:      "",
			Suggestions: []Suggestion{},
		},
		nil
	}

	result.Suggestions = sanitizeSuggestions(result.Suggestions)
	return &result, nil
}

func cleanJSON(content string) string {
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}

// 预编译正则，匹配常见的占位符模式
// 1. <...> : 尖括号占位符
// 2. [...] : 方括号占位符 (但需小心数组索引，这里主要匹配 [option] 这种)
// 3. path/to/ : 典型路径占位
// 4. your_ : 典型变量占位
var placeholderRegex = regexp.MustCompile(`<[^>]+>|[[a-zA-Z_]+]|\bpath/to/|\byour_`)

func sanitizeSuggestions(suggestions []Suggestion) []Suggestion {
	var clean []Suggestion
	for _, s := range suggestions {
		// 如果命令匹配到占位符正则，则丢弃
		if placeholderRegex.MatchString(s.Cmd) {
			continue
		}
		clean = append(clean, s)
	}
	return clean
}
