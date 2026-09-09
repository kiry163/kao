package ui

import (
	"os"

	"github.com/kiry163/kao/internal/recommend"
	"github.com/manifoldco/promptui"
)

// Select shows a thefuck-style list of recommended commands. Enter returns the
// chosen command (the caller fills it into the pane); Ctrl+C cancels and
// returns nil, nil. The list is rendered to stderr so stdout stays clean.
func Select(recommendations []recommend.Recommendation) (*recommend.Recommendation, error) {
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "▸ {{ .Command | cyan }}  {{ .Desc | faint }}",
		Inactive: "  {{ .Command }}  {{ .Desc | faint }}",
		Selected: "",
	}

	prompt := promptui.Select{
		Label:        "选择要填入的命令 (回车填充; 再次回车执行; Ctrl+C 取消)",
		Items:        recommendations,
		Templates:    templates,
		Size:         5,
		HideSelected: true,
		HideHelp:     true,
		Stdout:       os.Stderr,
	}

	i, _, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			return nil, nil
		}
		return nil, err
	}
	return &recommendations[i], nil
}
