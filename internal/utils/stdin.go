package utils

import (
	"io"
	"os"
)

func IsPiped() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	// 如果不是字符设备，说明有东西在管道里
	return (info.Mode() & os.ModeCharDevice) == 0
}

func ReadStdin() (string, error) {
	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
