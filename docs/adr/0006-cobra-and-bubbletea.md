# 0006 命令行解析用 cobra，交互界面用 bubbletea

状态：采纳

## 背景

`cmd/cc-login/main.go` 手写参数解析。后续要移植 8 个命令（含 `--` 之后透传给 claude 的参数、`--sign-in` / `--api-key` 等选项）、生成 zsh 补全，还想加一个交互式界面来浏览和切换 login。zsh 版的提问（`read -q` 确认、不回显读取 token）也要有对应实现。

## 决策

- 命令行用 `spf13/cobra`：每个命令一个 `cobra.Command`，cobra 负责分发命令和生成补全。
- 参数检查照 zsh 版 `cc-login()` 自己写（`cmd/cc-login/parse.go`），所有命令设 `DisableFlagParsing`：cobra 的 flag 解析允许选项出现在任意位置（`add --api-key NAME`），而 zsh 版要求选项紧跟在 NAME 后面、claude 的选项在 `--` 之后，报错文案也不同。`SilenceErrors` / `SilenceUsage`，自定义 help 命令和 `SetHelpFunc`，输出 `cc-login: unknown command x; the commands: ...` 这类原文，退出码一致，由 `cmd/cc-login/main_test.go` 锁住。
- 与 zsh 版的文案差异只有：usage 和命令列表加上 Go 独有的 `list --json`、`completion`；`run` / `renew` 的 "options for claude go after --" 报错不再建议它们不接受的 `--sign-in`。
- zsh 补全由 cobra 生成（`cc-login completion zsh`），login 名字由 `ValidArgsFunction` 列出，替代 zsh 版手写的 `_cc_login_complete`；zsh shim（0004）负责加载。
- 交互界面用 `charmbracelet/bubbletea` + `bubbles` + `lipgloss`，入口是单独的 `cc-login tui`：显示和 `list` 相同的信息，可选中一个 login 执行 use / run / renew / remove。
- `cc-login` 不带参数仍等于 `list`，纯文本输出与 zsh 版一致；TUI 只在显式调用、且 stdin 和 stdout 都是终端时启动，否则报错退出 1。
- 确认提问和不回显输入用 `charmbracelet/huh`，只在终端里用；非终端时与 zsh 版一样从 stdin 读，需要确认的命令提供 `--yes`。
- lipgloss 只用于 TUI；`list` 的颜色继续用现有的 ANSI 码，保证与 zsh 版逐字节一致。

## 后果

- 依赖变多，二进制变大几 MB；启动时间仍在毫秒级。
- cobra 的 flag 解析和默认报错都不用，只用它的命令分发和补全；参数检查是手写的，用测试锁住。
- TUI 没有 zsh 版可比对，靠 bubbletea 的模型单元测试（发送按键消息、检查状态和视图）。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
