package ui

import (
	"os"

	"github.com/kiry163/kao/internal/analyze"
	"github.com/manifoldco/promptui"
)

// Select shows a thefuck-style list of suggestions. Enter fills the chosen
// command into the pane's command line (no submit); Ctrl+C cancels and returns
// nil, nil. The list is rendered to stderr so stdout carries only the selected
// command in print mode.
func Select(suggestions []analyze.Suggestion) (*analyze.Suggestion, error) {
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "▸ {{ .Cmd | cyan }}  {{ .Desc | faint }}",
		Inactive: "  {{ .Cmd }}  {{ .Desc | faint }}",
		Selected: "",
	}

	prompt := promptui.Select{
		Label:        "选择要填入的命令 (回车填充; 再次回车执行; Ctrl+C 取消)",
		Items:        suggestions,
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
	return &suggestions[i], nil
}
