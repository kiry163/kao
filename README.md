# Kao (靠)

一个运行在 [herdr](https://herdr.dev) pane 内的 AI 命令推荐工具。在终端敲一下 `k`(可带自然语言目标),它读取当前 pane 的滚动缓冲,让 LLM 推荐你接下来该执行的命令——把修正拼写、下一步该做什么、"怎么把目录打包"这类问题变成可直接执行的命令,选中后填入命令行,你编辑确认后回车。

"靠!"——报错不用慌,靠一下就有答案。

## ✨ 特性

- **herdr 原生**:通过 `herdr` CLI 读取 pane 滚动缓冲、向 pane 注入文本(send-text 纯 prefill,不自动提交)。只认 `HERDR_ENV`/`HERDR_PANE_ID` 环境,必须运行在 herdr pane 内。
- **两种模式,一套结构**:
  - 上下文模式(`k`):最近命令失败则首推修正后的命令,成功则推合理的下一步。
  - 目标查询(`k 把当前改动提交并推送`):结合工作区快照与终端上下文,推荐达成目标的命令(可多条,按执行顺序)。
- **只推荐,不执行**:选中命令一律填入当前命令行,由你编辑(含 `<占位符>`)再回车。k 从不自动运行命令。
- **自清洗缓冲**:发送给模型前,程序先剔除 kao/k 自身痕迹——k 调用行、空提示符行、prefill 注入回显、UI 残留——并把快照切成「最近命令区 + 更早上下文」两段。当前会话的提示符从本次调用行自动推导,无需任何配置;识别不了的场景降级为保守清洗,不猜测边界。
- **工作区快照**(查询模式):模型能"看到"工作目录内容——优先 `tree -a -L 2`(忽略 `.git/node_modules/target/dist` 等),没有 tree 命令时退化为 `ls -la`;只读、限长、失败静默降级。
- **干净输出**:推荐过程与中间状态不打印,只有选择列表本身。

## 📦 安装

从 GitHub Releases 下载最新版本并安装到 `~/.local/bin/k`:

```bash
curl -fsSL https://raw.githubusercontent.com/kiry163/kao/main/install.sh | sh
```

需要 `curl`;安装目录可用 `INSTALL_DIR` 覆盖,指定版本可用 `KAO_VERSION`(如 `v0.1.0`)。确保安装目录在 PATH 中(`echo $PATH | grep ~/.local/bin`),之后在 herdr pane 内直接敲 `k`。

查看版本:`k -v`。

## ⚙️ 配置

配置文件位置(首次运行会自动创建默认文件):

- `~/.config/kao/config.yaml`

```yaml
provider: openai_compatible   # openai | openai_compatible | qwen | deepseek
api_key: ""                   # openai_compatible 可留空(本地模型)
model: gpt-4o-mini
base_url: http://127.0.0.1:11434/v1
thinking: false
snapshot_lines: 300
```

## 🚀 使用

```bash
$ git statts
git: 'statts' is not a git command. See 'git --help'.

$ k
选择要填入的命令 (回车填充; 再次回车执行; Ctrl+C 取消)
  ▸ git status  修正拼写:statts → status
```

选中后把 `git status` 填入命令行,你按回车执行。

### 目标查询

```bash
$ k 把当前改动提交并推送
选择要填入的命令 (回车填充; 再次回车执行; Ctrl+C 取消)
  ▸ git add -A && git commit -m "<COMMIT_MESSAGE>" && git push  提交并推送全部改动
```

只有你本人知道的值(提交信息、分支名等)用 `<英文大写>` 占位符表示,选中后在命令行里替换再回车。上下文模式(修复/下一步)仍然禁止占位符——修复命令必须拿来就能跑。

### 其他

- `k -v` 打印版本(`version`/`commit`/`date` 在发布构建中由 CI 注入)。
- 所有参数都是目标文本,无需引号;目标以 `-` 开头也可直接传(如 `k -rf /tmp`)。

## 📝 License

MIT
