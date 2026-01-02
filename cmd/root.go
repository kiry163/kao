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
		// 为了支持 eval 模式，所有非结果输出都必须走 Stderr
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

		// 1. 优先检查管道输入 (Pipe Mode) - 适用于所有用户
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
				fmt.Fprintf(logOut, "\n--- AI 建议 ---\n%s\n", analysis)
				return
			}
		}
		
		// 2. 尝试集成 Warp 终端 (参数 + DB 双重校验)
		var warpBlock *warp.Block
		// 只有在严格的 Warp 环境下才执行 DB 读取
		if warp.IsWarp() {
			debugLog("检测到 Warp 终端环境 (TERM_PROGRAM=WarpTerminal)")
			block, err := warp.GetLastCommand()
			if err == nil {
				warpBlock = block
				debugLog("已从 Warp DB 读取最新记录: [%s]", block.Command)
			} else {
				debugLog("Warp DB 读取未命中: %v", err)
			}
		} else {
			debugLog("当前非 Warp 环境，跳过 DB 读取")
		}

		// 决策: 是否使用 Warp 数据?
		useWarpData := false
		if warpBlock != nil {
			if len(args) == 0 {
				// 场景: Warp 用户直接输 'kao' (无参数) -> 信任 DB
				useWarpData = true
				debugLog("命中策略: 无参数，直接使用 Warp 历史")
			} else {
				// 场景: Warp 用户输 'k' (带参数) -> 校验 DB 是否匹配
				inputCmd := strings.TrimSpace(args[0])
				warpCmd := strings.TrimSpace(warpBlock.Command)
				
				// 对比 Shell 传来的命令和 DB 里的命令是否一致
				// 如果一致，说明我们拥有了该命令的 Output，无需重放
				if inputCmd == warpCmd {
					useWarpData = true
					debugLog("命中策略: 输入参数与 Warp 历史一致，优先使用 Warp 数据(含Output)")
				} else {
					debugLog("策略跳过: 输入参数 (%s) 与 Warp 历史 (%s) 不一致，转为手动模式", inputCmd, warpCmd)
				}
			}
		}

		// 分支 A: 使用 Warp 数据 (零重放)
		if useWarpData {
			debugLog("--------------------------------------------------")
			debugLog("执行 Warp 零重放分析:")
			debugLog("COMMAND  : %s", warpBlock.Command)
			debugLog("EXIT CODE: %d", warpBlock.ExitCode)
			debugLog("OUTPUT   :\n%s", warpBlock.Output)
			debugLog("--------------------------------------------------")

			fmt.Fprintf(logOut, "🔍 从 Warp 历史中读取命令: %s\n", warpBlock.Command)
			fmt.Fprintln(logOut, "🤖 正在分析输出日志...")
			
			analysis, err := client.AnalyzeError(ctx, warpBlock.Command, warpBlock.Output)
			if err != nil {
				fmt.Fprintf(logOut, "AI 分析失败: %v\n", err)
				return
			}
			fmt.Fprintf(logOut, "\n--- AI 建议 ---\n%s\n", analysis)
			return
		}

		// 分支 B: 传统模式 (Fallback)
		// 适用于: 非 Warp 用户，或者 Warp 数据没匹配上的情况
		if len(args) == 0 {
			debugLog("未检测到命令参数，且未使用 Warp 数据")
			fmt.Fprintln(logOut, "提示: 请输入要分析的命令，或通过管道传入日志。")
			fmt.Fprintln(logOut, "建议在 Shell 配置文件中添加别名:")
			fmt.Fprintln(logOut, "alias k='kao --fix \"$(fc -ln -1)\"'")
			return
		}

		lastCmd := args[0]
		debugLog("进入通用分析模式 (Audit+Replay)，目标命令: %s", lastCmd)
		fmt.Fprintf(logOut, "🔍 正在分析命令意图: %s\n", lastCmd)

		intent, err := client.AnalyzeIntent(ctx, lastCmd)
		if err != nil {
			fmt.Fprintf(logOut, "分析失败: %v\n", err)
			return
		}
		debugLog("AI 意图分析结果: Type=%s, Safe=%v, Suggestions=%v", intent.Type, intent.SafeToRun, intent.Suggestions)

		// 场景 B-1: 明显的拼写错误 (Typo)
		if intent.Type == "typo" && len(intent.Suggestions) > 0 {
			debugLog("触发 Typo 修正流程")
			if fixMode {
				handleFixSelection(intent.Suggestions)
			} else {
				fmt.Fprintf(logOut, "💡 这是一个拼写错误，建议修正为:\n")
				for _, s := range intent.Suggestions {
					fmt.Fprintf(logOut, " - %s\n", s)
				}
				fmt.Fprintln(logOut, "\n(提示: 更新 Shell 函数配置可开启交互式修正，详见 README)")
			}
			return
		}

		// 场景 B-2: 包含风险，不允许自动运行
		if !intent.SafeToRun {
			debugLog("命令被拦截: Unsafe")
			fmt.Fprintf(logOut, "⚠️  AI 判定该命令存在风险 (%s): %s\n", intent.Type, intent.Reason)
			fmt.Fprintln(logOut, "为了安全起见，Kao 不会自动重新执行该命令。")
			return
		}

		// 场景 B-3: 需要运行获取报错信息 (Logic Error)
		debugLog("开始执行命令重放...")
		fmt.Fprintln(logOut, "✅ 审计通过，正在后台重运行命令捕获输出...")
		
		startTime := time.Now()
		execResult, err := executor.ExecuteCommand(ctx, lastCmd)
		duration := time.Since(startTime)
		
		if err != nil {
			fmt.Fprintf(logOut, "执行命令失败: %v\n", err)
			return
		}
		debugLog("命令执行完成，耗时: %v, ExitCode: %d, OutputLength: %d", duration, execResult.ExitCode, len(execResult.Output))
		
		if debugMode && len(execResult.Output) > 0 {
			preview := execResult.Output
			if len(preview) > 500 {
				preview = preview[:500] + "...(truncated)"
			}
			debugLog("Output Preview: %q", preview)
		}

		if execResult.ExitCode == 0 {
			debugLog("命令 ExitCode=0，流程终止")
			fmt.Fprintln(logOut, "🎉 该命令执行成功，似乎没有错误。")
			return
		}

		fmt.Fprintln(logOut, "🤖 正在结合输出进行 AI 分析...")
		analysis, err := client.AnalyzeError(ctx, lastCmd, execResult.Output)
		if err != nil {
			fmt.Fprintf(logOut, "AI 分析失败: %v\n", err)
			return
		}

		fmt.Fprintf(logOut, "\n--- AI 建议 ---\n%s\n", analysis)
	},
}

// handleFixSelection 处理交互式选择
func handleFixSelection(suggestions []string) {
	prompt := promptui.Select{
		Label: "检测到拼写错误，请选择修正后的命令执行 (Ctrl+C 取消)",
		Items: suggestions,
		Stdout: os.Stderr,
	}

	_, result, err := prompt.Run()

	if err != nil {
		if err == promptui.ErrInterrupt {
			debugLog("用户取消了选择 (Ctrl+C)")
			return
		}
		fmt.Fprintf(os.Stderr, "选择失败 %v\n", err)
		return
	}

	debugLog("用户选择了修正命令: %s", result)
	fmt.Print(result)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
