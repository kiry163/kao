package herdr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
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

// waitForPromptTimeout bounds how long a deferred send waits for the shell
// prompt; after that the prefill is abandoned.
const waitForPromptTimeout = 10 * time.Second

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

// SendText fills the pane's command line with text without submitting it
// (prefill), via a detached child that first waits for the shell to return to
// its prompt (the last rendered line contains waitFor). The wait exists
// because k is the pane's foreground process: a synchronous send-text would
// deliver the bytes while k still owns the foreground, racing its exit instead
// of landing on the fresh prompt line; k exits right after spawning the child,
// so the wait is normally instantaneous. With an empty waitFor the text is
// sent immediately as a fallback.
func (c *Client) SendText(paneID, text, waitFor string) error {
	if waitFor == "" {
		ctx, cancel := context.WithTimeout(context.Background(), paneTimeout)
		defer cancel()
		if _, err := c.run(ctx, "pane", "send-text", paneID, text); err != nil {
			return fmt.Errorf("发送文本失败: %s", err)
		}
		return nil
	}

	script := fmt.Sprintf("%s pane wait-output %s --match %s --lines 1 --timeout %d && %s pane send-text %s %s",
		shQuote(c.bin), shQuote(paneID), shQuote(waitFor), waitForPromptTimeout/time.Millisecond,
		shQuote(c.bin), shQuote(paneID), shQuote(text))

	cmd := exec.Command("/bin/sh", "-c", script)
	// Detach: the child must outlive k (which exits right after spawning it)
	// and must not die with k's process group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// The child's output would otherwise print into the pane once k is gone
	// (e.g. a wait-output timeout error); it is purely a side effect.
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("打开 %s 失败: %w", os.DevNull, err)
	}
	defer devNull.Close()
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动延迟写入失败: %w", err)
	}
	go cmd.Wait() // reap the child; its success is unobservable by design
	return nil
}

// shQuote wraps s in single quotes for use inside a /bin/sh -c script,
// escaping embedded single quotes (' -> '\”).
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
