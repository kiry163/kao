# Kao (靠)

一个运行在 [herdr](https://herdr.dev) pane 内的 AI 命令修复工具。输错命令时,在终端跑一下 `k`,它读取当前 pane 的滚动缓冲,让 LLM 定位上一条真实命令,给出可执行的修复建议,你选中后填入命令行或直接执行。

名字源于程序员看到命令报错时那句 "靠!"。

## ✨ 特性

- **herdr 原生**:通过 `herdr` CLI 读取 pane 滚动缓冲、向 pane 注入文本,无重放。只认 `HERDR_ENV`/`HERDR_PANE_ID` 环境,必须运行在 herdr pane 内。
- **AI 诊断**:快照末尾是用户输入的 `k`/`kao` 行,模型识别并忽略它,分析其上一条真实命令及其输出,判断成败并给出 0-3 条建议(每条完整、可直接执行、无占位符)。
- **thefuck 式选择**:promptui 交互列表,Enter 选定,Ctrl+C 取消。选中后默认把命令**填入**当前命令行(prefill,等你回车);可配置为直接执行。
- **干净输出**:分析过程与"识别到命令"等状态信息默认不打印,只有选择列表本身。选中后不打印 `✓ {...}` 确认行。

## 📦 安装

### 源码编译

```bash
git clone https://github.com/kiry163/kao.git
cd kao
go build -o k
```

`k` 可执行文件建议放在 PATH 下(`mv k /usr/local/bin/`),或直接 `./k` 使用。

## ⚙️ 配置

配置文件位置(首次运行会自动创建默认文件):

- `$XDG_CONFIG_HOME/kao/config.yaml`,未设置时 `~/.config/kao/config.yaml`

```yaml
provider: openai_compatible   # openai | openai_compatible | qwen | deepseek
api_key: ""                   # openai_compatible 可留空(本地模型)
model: gpt-4o-mini
base_url: http://127.0.0.1:11434/v1
thinking: false
snapshot_lines: 300
auto_execute: false           # true: 选中后直接执行,默认 false 为填入命令行
```

## 🚀 使用

```bash
$ git statts
git: 'statts' is not a git command. See 'git --help'.

$ k
选择要填入的命令 (回车填充; 再次回车执行; Ctrl+C 取消)
  ▸ git status  修正 git statts -> git status
```

选中后默认把 `git status` 填入命令行,你按回车执行。

### 直接执行

要选中后直接执行命令,把配置文件里的 `auto_execute` 设为 `true`,或临时加 `-auto-execute`:

```bash
k -auto-execute
```

### 消除 PTY 回显

默认的"填入命令行"由 `k` 通过 `herdr pane send-text` 注入。因运行 `k` 时终端处于 canonical+ECHO 模式,注入的命令会在滚动缓冲里被回显一行(命令本身,非重复执行)。若想彻底干净,用 `k init` 生成一个 shell 函数:

```bash
k init   # 依 SHELL 写入 ~/.zshrc 或 ~/.bashrc,幂等
```

函数走 `k --print`(选中命令打到 stdout,列表走 stderr)+ zsh `print -z` / bash `READLINE_LINE`,把命令直接填入当前输入缓冲区,**无 PTY 回显**。使用后直接敲 `k`,而非 `./k`。

### 其他

- `k --print`:选中命令打印到 stdout、列表走 stderr(供 shell 函数)。
- `k -d`:调试日志(`[kao]` 前缀,输出到 stderr)。
- `k --lines N`:读取 pane 行数,覆盖 `snapshot_lines`。
- `k init`:写入 shell 函数(参见上文)。

## 📝 License

MIT
