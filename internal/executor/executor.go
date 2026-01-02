package executor

import (
	"bytes"
	"context"
	"os/exec"
)

type ExecuteResult struct {
	Output   string
	ExitCode int
}

func ExecuteCommand(ctx context.Context, cmdStr string) (*ExecuteResult, error) {
	// 使用 /bin/sh -c 来执行命令，这样可以支持复杂的管道和 shell 特性
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	
	var combinedOutput bytes.Buffer
	cmd.Stdout = &combinedOutput
	cmd.Stderr = &combinedOutput

	err := cmd.Run()
	
	result := &ExecuteResult{
		Output: combinedOutput.String(),
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		} else {
			return nil, err
		}
	} else {
		result.ExitCode = 0
	}

	return result, nil
}
