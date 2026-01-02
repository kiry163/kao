package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"kao/internal/config"
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

type IntentAnalysis struct {
	Type        string   `json:"type"`        // "typo" (拼写错误), "logic" (逻辑错误/需要报错信息), "unsafe" (不安全)
	Suggestions []string `json:"suggestions"` // 修正后的命令建议列表 (仅当 Type="typo" 时)
	Reason      string   `json:"reason"`      // 分析理由
	SafeToRun   bool     `json:"safeToRun"`   // 原命令是否安全可重跑 (用于获取报错信息)
}

func (c *Client) AnalyzeIntent(ctx context.Context, cmd string) (*IntentAnalysis, error) {
	prompt := fmt.Sprintf(`你是一个智能终端助手。请分析用户输入的命令，判断其意图和安全性。

命令: %s

请返回 JSON 格式结果，包含以下字段：
1. "type": 
   - "typo": 如果是明显的拼写错误 (如 'git brnch')。
   - "logic": 如果命令拼写正确但可能参数错误，需要运行获取报错才能分析。
   - "unsafe": 如果命令具有破坏性 (rm, mv, dd, shutdown) 或副作用 (curl POST)。
2. "suggestions": 字符串数组。如果是 "typo"，请列出 1-3 个修正后的命令建议。否则为空。
3. "safeToRun": 布尔值。如果命令是只读的或为了获取报错信息而重运行是安全的，则为 true；否则为 false。
4. "reason": 简短中文说明。

例如: {"type": "typo", "suggestions": ["git branch"], "safeToRun": false, "reason": "检测到拼写错误"}
例如: {"type": "logic", "suggestions": [], "safeToRun": true, "reason": "命令看似正确，需运行获取报错"}

请严格返回 JSON，不要包含 Markdown 标记。`, cmd)

	resp, err := c.openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.config.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})

	if err != nil {
		return nil, err
	}

	content := resp.Choices[0].Message.Content
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result IntentAnalysis
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// 降级策略: 如果解析失败，默认当作不安全
		return &IntentAnalysis{Type: "unsafe", SafeToRun: false, Reason: "AI返回格式异常"}, nil
	}

	return &result, nil
}

func (c *Client) AnalyzeError(ctx context.Context, cmd, output string) (string, error) {
	prompt := fmt.Sprintf(`你是一个资深的软件工程师。用户在终端执行了一个命令，但似乎遇到了问题。请根据命令和输出结果进行分析，并提供简洁明了的解决方案。

执行命令: 
%s

执行输出:
%s

请直接给出分析结果和修复建议，使用中文。`, cmd, output)

	resp, err := c.openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.config.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})

	if err != nil {
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}
