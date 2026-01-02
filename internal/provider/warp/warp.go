package warp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "modernc.org/sqlite" // Register driver
)

type Block struct {
	Command  string
	Output   string
	ExitCode int
}

// WarpTextNode 尝试匹配 Warp 可能的 JSON 结构
type WarpTextNode struct {
	Text string `json:"text"`
}

func GetDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library/Group Containers/2BBY89MBSN.dev.warp/Library/Application Support/dev.warp.Warp-Stable/warp.sqlite"), nil
	case "linux":
		// 根据 XDG_STATE_HOME 规范
		stateHome := os.Getenv("XDG_STATE_HOME")
		if stateHome == "" {
			stateHome = filepath.Join(home, ".local/state")
		}
		return filepath.Join(stateHome, "warp-terminal/warp.sqlite"), nil
	// TODO: Windows 支持需要确认路径格式
	default:
		return "", fmt.Errorf("unsupported OS for Warp integration")
	}
}

func IsWarp() bool {
	// 严格模式: 仅当 TERM_PROGRAM 明确为 WarpTerminal 时才认为是 Warp 环境
	// 避免用户安装了 Warp 但在 Kitty/iTerm2 中使用 kao 时误读 Warp 历史
	return os.Getenv("TERM_PROGRAM") == "WarpTerminal"
}

// GetLastCommand 获取最近一条非 kao 的命令
func GetLastCommand() (*Block, error) {
	dbPath, err := GetDBPath()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// 优化说明:
	// Warp 的 stylized_command 存储的是 JSON 格式的 BLOB 数据 (如 [{"text":"cmd..."}])。
	// 在 SQL 层使用 LIKE '%kao%' 进行过滤非常危险且不可靠，因为无法正确处理 JSON 结构。
	// 因此，最佳策略是取出最近的少量记录 (LIMIT 20)，在应用层解析 JSON 后进行精确过滤。
	// 这种做法在性能 (SQLite 极快) 和准确性之间取得了最好的平衡。
	rows, err := db.Query(`
		SELECT stylized_command, stylized_output, exit_code 
		FROM blocks 
		WHERE did_execute = 1 
		ORDER BY id DESC 
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var rawCmd, rawOut string
		var exitCode int
		if err := rows.Scan(&rawCmd, &rawOut, &exitCode); err != nil {
			continue
		}

		// 解析内容 (尝试清洗 JSON)
		cmdText := cleanText(rawCmd)
		
		// 过滤规则:
		// 1. 排除空命令
		// 2. 排除以 "k" 或 "kao" 开头的命令 (防止递归分析自己)
		trimmed := strings.TrimSpace(cmdText)
		if trimmed == "" {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) > 0 {
			prog := parts[0]
			// 检查命令是否为 kao 自身
			if prog == "k" || prog == "kao" || strings.HasSuffix(prog, "/kao") {
				continue
			}
		}

		return &Block{
			Command:  cmdText,
			Output:   cleanText(rawOut),
			ExitCode: exitCode,
		}, nil
	}

	return nil, fmt.Errorf("no suitable command found in history")
}

// cleanText 尝试将 Warp 的 stylized 文本转换为纯文本
func cleanText(raw string) string {
	// 1. 如果是简单的纯文本，直接返回
	if !strings.HasPrefix(raw, "[") && !strings.HasPrefix(raw, "{") {
		return raw
	}

	// 2. 尝试解析 JSON 数组
	var nodes []WarpTextNode
	if err := json.Unmarshal([]byte(raw), &nodes); err == nil {
		var sb strings.Builder
		for _, node := range nodes {
			sb.WriteString(node.Text)
		}
		return sb.String()
	}

	// 3. 降级: 返回原始值
	return raw
}