# Kao (靠) - 你的智能终端副驾驶 🤖

**Kao** (读作 "靠") 是一个基于 AI 的终端命令行伴侣。当你遇到命令报错、拼写错误或者不知所措时，只需要敲一下 `k`，它就能帮你分析原因、修复错误甚至自动执行修正后的命令。

它不仅仅是一个简单的 "wrapper"，更是一个集成了 **AI 安全审计**、**Warp 终端原生支持** 和 **交互式修复** 的生产力工具。

## ✨ 核心特性

- **⚡️ Warp 终端原生集成**: 如果你在使用 [Warp](https://www.warp.dev/) 终端，Kao 会直接读取其内部数据库获取报错信息。**零重放、零副作用、100% 精确**。
- **🛡️ AI 安全审计**: 对于普通终端，Kao 在后台重运行命令前，会先让 AI 判断该命令是否安全。危险命令 (如 `rm`, `mv`, 非幂等 API) 会被自动拦截。
- **🔧 交互式智能修复**: 检测到拼写错误 (Typo) 时，直接弹出交互式菜单，按回车即可立即修正并执行。
- **🔍 多模式支持**: 
  - **Auto Mode**: 自动获取上一条命令分析。
  - **Pipe Mode**: 支持 `command |& kao` 手动投喂日志。

## 📦 安装

### 源码编译

```bash
# 1. 克隆项目
git clone https://github.com/your-username/kao.git
cd kao

# 2. 编译
go build -o kao

# 3. 移动到 PATH 路径下
mv kao /usr/local/bin/
```

## ⚙️ 配置

Kao 依赖 OpenAI 兼容的 API (如 OpenAI, Azure, DeepSeek 等)。请在你的 `.zshrc` 或 `.bashrc` 中设置以下环境变量：

```bash
export KAO_API_KEY="sk-xxxxxxxxxxxxxxxx"
export KAO_BASE_URL="https://api.openai.com/v1" # 可选，默认为 OpenAI 官方
export KAO_MODEL="gpt-3.5-turbo"                # 可选，默认为 gpt-3.5-turbo
```

## 🚀 Shell 集成 (强烈推荐)

为了获得最佳体验（如自动获取上个命令、交互式执行修正命令），请将以下函数添加到你的 Shell 配置文件中 (`~/.zshrc` 或 `~/.bashrc`)。

### Zsh / Bash 配置

```bash
# === Kao 快捷指令 ===
function k() {
    # 1. 获取上一条实际执行的命令 (排除 k/kao 自身)
    local LAST_CMD=""
    if [ -n "$ZSH_VERSION" ]; then
        # Zsh 适配
        LAST_CMD=$(fc -ln -10 | grep -vE '^\s*(k|kao)\b' | tail -1)
    else
        # Bash 适配
        LAST_CMD=$(history | tail -n 10 | grep -vE '^\s*[0-9]+\s+(k|kao)\b' | tail -n 1 | sed 's/^\s*[0-9]\+\s\+//')
    fi

    # 2. 调用 kao 进入交互模式
    #    "$@" 允许传递参数 (如 k -d)
    #    kao 的交互界面输出到 stderr，最终选定的命令输出到 stdout
    local FIXED_CMD=$(kao --fix "$@" "$LAST_CMD")

    # 3. 如果 Kao 返回了修正后的命令，则在当前 Shell 执行
    if [ -n "$FIXED_CMD" ]; then
        # 将修正后的命令加入历史记录
        if [ -n "$ZSH_VERSION" ]; then
            print -s "$FIXED_CMD"
        else
            history -s "$FIXED_CMD"
        fi
        
        echo -e "\n\033[1;32mRunning: $FIXED_CMD\033[0m"
        eval "$FIXED_CMD"
    fi
}
```

配置完成后，记得执行 `source ~/.zshrc` 或重启终端。

## 📖 使用指南

### 1. 自动分析 (最常用)

当你执行命令报错时，直接输入 `k`：

```bash
$ git brnch
git: 'brnch' is not a git command. See 'git --help'.

$ k
? 检测到拼写错误，请选择修正后的命令执行: 
> git branch
  ❌ Cancel
```

或者遇到运行时错误：

```bash
$ mvn install
... BUILD FAILURE ...

$ k
🔍 正在分析命令意图: mvn install
✅ 审计通过，正在后台重运行命令捕获输出...
...
--- AI 建议 ---
构建失败是因为缺少依赖包 xxxx，建议执行...
```

### 2. 调试模式

如果你想知道 Kao 到底在做什么（或者是否成功读取了 Warp 数据），加上 `-d` 参数：

```bash
$ k -d
[DEBUG 10:00:00] 检测到 Warp 终端环境...
[DEBUG 10:00:00] 成功从 Warp DB 读取记录...
```

### 3. 管道模式 (手动挡)

对于某些极其敏感或复杂的场景，你可以手动将日志传给 Kao：

```bash
# |& 同时传递 stdout 和 stderr
$ ./dangerous_script.sh |& kao
```

## 🛠️ 技术原理

Kao 采用 **双源校验 (Dual-Source Validation)** 策略来确保兼容性与安全性：

1.  **环境检测**: 自动识别当前是否为 Warp 终端 (`TERM_PROGRAM=WarpTerminal`)。
2.  **策略分发**: 
    *   **Warp 环境**: 优先读取 Warp 内部 SQLite 数据库 (`warp.sqlite`)。如果 Shell 传入的参数与 DB 记录一致，直接复用 DB 中的 Output，实现 **零重放分析**。
    *   **普通环境**: 接收 Shell 传入的上一条命令，通过 AI 进行 **安全审计** (判断是否为 `rm`, `mv` 等高风险操作)。安全则后台静默重放抓取 Output，不安全则拦截。
3.  **交互执行**: 利用 `eval` 机制，让 Go 程序只负责计算修正命令，由 Shell 负责最终执行，完美解决环境变量和历史记录问题。

## 📝 License

MIT

```