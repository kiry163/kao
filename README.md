# Kao (靠) - 你的智能终端副驾驶 🤖

**Kao** (读作 "靠") 是一个基于 AI 的终端命令行伴侣。这个名字源于程序员在遇到 Bug 或命令报错时最常说的那个词："靠！"。现在，当你再想说这个词的时候，只需要敲一下 `k`，它就能帮你分析原因、修复错误甚至自动执行修正后的命令。

它不仅仅是一个简单的 "wrapper"，更是一个集成了 **AI 安全审计**、**Warp 终端原生支持** 和 **交互式修复** 的生产力工具。

## ✨ 核心特性

- **⚡️ Warp 终端原生集成**: 如果你在使用 [Warp](https://www.warp.dev/) 终端，Kao 会直接读取其内部数据库获取报错信息。**零重放、零副作用、100% 精确**。
- **🛡️ AI 安全审计**: 对于普通终端，Kao 在后台重运行命令前，会先让 AI 判断该命令是否安全。危险命令 (如 `rm`, `mv`, 非幂等 API) 会被自动拦截。
- **🧠 智能预测与建议**: 
  - **出错时**: 给出详细原因分析 + 自然语言指导 (Advice) + 一键修复命令 (Suggestions)。
  - **成功时**: 解释执行结果 + 预测你可能想执行的后续命令 (如 `mkdir` -> `cd`)。
- **🌍 系统环境感知**: AI 会根据你的操作系统 (macOS/Linux) 推荐最原生的工具 (如 `brew` vs `apt`)。
- **🔧 交互式智能修复**: 
  - 漂亮的交互列表，包含命令功能注释。
  - 支持 `Ctrl+C` 快捷取消。

## 📦 安装

### 源码编译

```bash
# 1. 克隆项目
git clone https://github.com/zqr233qr/kao.git
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

### 1. 自动分析与修复 (最常用)

**场景 A: 拼写错误**
```bash
$ git brnch
git: 'brnch' is not a git command. See 'git --help'.

$ k
? 请选择建议的命令 (Ctrl+C 取消)
▸ git branch  修正拼写错误 (推荐)
```

**场景 B: 缺少依赖 (智能建议)**
```bash
$ cargo run
error: no command named `cargo` found...

$ k
--- AI 分析 ---
系统未找到 cargo 命令，可能是 Rust 环境未安装。

💡 建议: 请根据您的网络环境选择合适的安装方式。

? 请选择建议的命令 (Ctrl+C 取消)
▸ curl --proto '=https' ... | sh   官方脚本安装 Rust
  brew install rust                通过 Homebrew 安装 (macOS)
```

**场景 C: 成功后的预测**
```bash
$ mkdir my-project

$ k
--- AI 分析 ---
目录 'my-project' 创建成功。

? 请选择建议的命令 (Ctrl+C 取消)
▸ cd my-project   进入新创建的目录
  ls -l           查看目录权限
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

## 📝 License

MIT

```