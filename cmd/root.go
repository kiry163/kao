package cmd

import (
	"context"
	"fmt"
	"kao/internal/ai"
	"kao/internal/config"
	"kao/internal/executor"
	"kao/internal/provider/warp"
	"kao/internal/utils"
	"os"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	fixMode   bool
	debugMode bool
)

func init() {
	rootCmd.Flags().BoolVar(&fixMode, "fix", false, "开启交互式修复模式 (输出仅包含修正后的命令)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "开启调试模式，打印详细日志")
}

func debugLog(format string, args ...interface{}) {
	if debugMode {
		prefix := fmt.Sprintf("\033[36m[DEBUG %s]\033[0m ", time.Now().Format("15:04:05"))
		fmt.Fprintf(os.Stderr, prefix+format+"\n", args...)
	}
}

var rootCmd = &cobra.Command{
	Use:   "kao [command]",
	Short: "Kao (靠) - 你的智能终端副驾驶",
	Long: `Kao (读作 "靠") 源于程序员遇到报错时最常说的那个词。
它是一个基于 AI 的终端伴侣，可以自动捕获上一个命令的错误输出，并结合 AI 给出修复建议。
支持直接运行 (自动分析上一个命令) 或 通过管道接收输出分析。`,
	Run: func(cmd *cobra.Command, args []string) {
		logOut := os.Stderr
		if !fixMode {
			logOut = os.Stdout
		}

		cfg := config.LoadConfig()
		debugLog("配置已加载: Model=%s, BaseURL=%s", cfg.Model, cfg.BaseURL)

		if cfg.APIKey == "" {
			fmt.Fprintln(logOut, "错误: 请设置 KAO_API_KEY 环境变量")
			os.Exit(1)
		}

		client := ai.NewClient(cfg)
		ctx := context.Background()

		// Helper to display analysis result
		displayAnalysis := func(analysis *ai.ErrorAnalysis) {
			fmt.Fprintf(logOut, "\n--- AI 分析 ---\n%s\n", analysis.Explanation)
			if analysis.Advice != "" {
				fmt.Fprintf(logOut, "\n💡 建议: %s\n", analysis.Advice)
			}
			if len(analysis.Suggestions) > 0 && fixMode {
				handleFixSelection(analysis.Suggestions)
			}
		}

		// 1. Pipe Mode
		if utils.IsPiped() {
			debugLog("检测到 Stdin 管道输入")
			stdinContent, err := utils.ReadStdin()
			if err != nil {
				fmt.Fprintf(logOut, "读取管道输入失败: %v\n", err)
				return
			}
			
			if stdinContent != "" {
				fmt.Fprintln(logOut, "🔍 检测到管道输入，正在分析日志...")
				analysis, err := client.AnalyzeError(ctx, "通过管道传入的日志", stdinContent)
				if err != nil {
					fmt.Fprintf(logOut, "AI 分析失败: %v\n", err)
					return
				}
				displayAnalysis(analysis)
				return
			}
		}
		
		// 2. Warp Mode
		var warpBlock *warp.Block
		if warp.IsWarp() {
			debugLog("检测到 Warp 终端环境 (TERM_PROGRAM=WarpTerminal)")
			block, err := warp.GetLastCommand()
			if err == nil {
				warpBlock = block
				debugLog("已从 Warp DB 读取最新记录: [%s]", block.Command)
			}
		}

		useWarpData := false
		if warpBlock != nil {
			if len(args) == 0 {
				useWarpData = true
			} else {
				inputCmd := strings.TrimSpace(args[0])
				warpCmd := strings.TrimSpace(warpBlock.Command)
				if inputCmd == warpCmd {
					useWarpData = true
				}
			}
		}

		if useWarpData {
			debugLog("执行 Warp 零重放分析")
			fmt.Fprintf(logOut, "🔍 从 Warp 历史中读取命令: %s\n", warpBlock.Command)
			fmt.Fprintln(logOut, "🤖 正在分析输出日志/预测...")
			
			analysis, err := client.AnalyzeError(ctx, warpBlock.Command, warpBlock.Output)
			if err != nil {
				fmt.Fprintf(logOut, "AI 分析失败: %v\n", err)
				return
			}
			displayAnalysis(analysis)
			return
		}

		// 3. Normal Mode
		if len(args) == 0 {
			fmt.Fprintln(logOut, "提示: 请输入要分析的命令，或通过管道传入日志。 ")
			return
		}

		lastCmd := args[0]
		debugLog("进入通用分析模式 (Audit+Replay): %s", lastCmd)
		fmt.Fprintf(logOut, "🔍 正在分析命令意图: %s\n", lastCmd)

		intent, err := client.AnalyzeIntent(ctx, lastCmd)
		if err != nil {
			fmt.Fprintf(logOut, "意图分析失败: %v\n", err)
			return
		}

		// Typo Quick Fix
		if intent.Type == "typo" && len(intent.Suggestions) > 0 {
			debugLog("触发快速 Typo 修正流程")
			// Intent 分析也可以有 Advice，打印出来
			if intent.Advice != "" {
				fmt.Fprintf(logOut, "💡 建议: %s\n", intent.Advice)
			}
			
			if fixMode {
				handleFixSelection(intent.Suggestions)
			} else {
				fmt.Fprintf(logOut, "💡 这是一个拼写错误，建议修正为:\n")
				for _, s := range intent.Suggestions {
					fmt.Fprintf(logOut, " - %s  # %s\n", s.Cmd, s.Desc)
				}
			}
			return
		}

		if !intent.SafeToRun {
			fmt.Fprintf(logOut, "⚠️  AI 判定该命令存在风险 (%s): %s\n", intent.Type, intent.Reason)
			return
		}

		fmt.Fprintln(logOut, "✅ 审计通过，正在后台重运行命令捕获输出...")
		execResult, err := executor.ExecuteCommand(ctx, lastCmd)
		if err != nil {
			fmt.Fprintf(logOut, "执行命令失败: %v\n", err)
			return
		}

		if execResult.ExitCode == 0 {
			debugLog("命令 ExitCode=0，继续流程以生成后续建议")
			fmt.Fprintln(logOut, "🎉 该命令执行成功，正在分析后续建议...")
		}

		fmt.Fprintln(logOut, "🤖 正在结合输出进行 AI 分析/预测...")
		analysis, err := client.AnalyzeError(ctx, lastCmd, execResult.Output)
		if err != nil {
			fmt.Fprintf(logOut, "AI 分析失败: %v\n", err)
			return
		}
		
		displayAnalysis(analysis)
	},
}

func handleFixSelection(suggestions []ai.Suggestion) {
	// 定义模板
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "▸ {{ .Cmd | cyan }}  {{ .Desc | faint }}",
		Inactive: "  {{ .Cmd }}  {{ .Desc | faint }}",
		Selected: "⚡️ 已选择: {{ .Cmd | green }}",
	}

	prompt := promptui.Select{
		Label:     "请选择建议的命令 (Ctrl+C 取消)",
		Items:     suggestions,
		Templates: templates,
		Stdout:    os.Stderr, 
		Size:      5,
	}

	i, _, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			debugLog("用户取消了选择 (Ctrl+C)")
			return
		}
		fmt.Fprintf(os.Stderr, "选择失败 %v\n", err)
		return
	}

	selected := suggestions[i]
	debugLog("用户选择了命令: %s (%s)", selected.Cmd, selected.Desc)
	
	fmt.Print(selected.Cmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
