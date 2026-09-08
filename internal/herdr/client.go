package herdr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	bin string
}

// New resolves the herdr binary on PATH.
func New() (*Client, error) {
	bin, err := exec.LookPath("herdr")
	if err != nil {
		return nil, fmt.Errorf("herdr 不在 PATH,无法运行")
	}
	return &Client{bin: bin}, nil
}

// EnvOK reports whether the process runs inside a herdr environment.
func (c *Client) EnvOK() bool {
	return os.Getenv("HERDR_ENV") == "1"
}

// EnvPaneID returns the id of the pane this process runs in.
func (c *Client) EnvPaneID() (string, error) {
	id := os.Getenv("HERDR_PANE_ID")
	if id == "" {
		return "", fmt.Errorf("缺少 HERDR_PANE_ID")
	}
	return id, nil
}

const paneTimeout = 30 * time.Second

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

// ReadPane reads up to lines lines from the pane's recent unwrapped scrollback.
// Empty stdout is not an error (an empty pane is a valid snapshot).
func (c *Client) ReadPane(paneID string, lines int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), paneTimeout)
	defer cancel()
	out, err := c.run(ctx, "pane", "read", paneID,
		"--source", "recent-unwrapped", "--lines", strconv.Itoa(lines))
	if err != nil {
		return "", fmt.Errorf("读取 pane 失败: %s", err)
	}
	return strings.TrimSpace(out), nil
}

// SendText injects text into the pane's command line without submitting it.
// No trailing Enter is sent; the shell shows it as a pending line (prefill).
func (c *Client) SendText(paneID, text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), paneTimeout)
	defer cancel()
	if _, err := c.run(ctx, "pane", "send-text", paneID, text); err != nil {
		return fmt.Errorf("发送文本失败: %s", err)
	}
	return nil
}

// RunPane sends command plus Enter to the pane's shell, executing it. The
// command runs as a normal shell command (auto-execute): no prefill, no PTY
// echo of a dangling command line.
func (c *Client) RunPane(paneID, command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), paneTimeout)
	defer cancel()
	if _, err := c.run(ctx, "pane", "run", paneID, command); err != nil {
		return fmt.Errorf("执行命令失败: %s", err)
	}
	return nil
}
