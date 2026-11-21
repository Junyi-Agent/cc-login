# 0006 命令行解析用 cobra，交互界面用 bubbletea

状态：采纳

## 背景

`cmd/cc-login/main.go` 手写参数解析。后续要移植 8 个命令（含 `--` 之后透传给 claude 的参数、`--sign-in` / `--api-key` 等选项）、生成 zsh 补全，还想加一个交互式界面来浏览和切换 login。zsh 版的提问（`read -q` 确认、不回显读取 token）也要有对应实现。

## 决策

- 命令行用 `spf13/cobra`，按 cobra 的常规用法：每个命令一个 `cobra.Command`，选项用 pflag 注册，参数检查用 `Args`（`ExactArgs`、`NoArgs` 和少量自定义校验），`--` 之后透传给 claude 的参数用 `ArgsLenAtDash` 判断。`add` 的 `--sign-in` / `--api-key` / `--oauth-token` 用 `MarkFlagsMutuallyExclusive`。
- help、用法和参数报错用 cobra 生成的，不再要求与 zsh 版逐字节一致（用户 2026-11-21 决定）。报错统一是 `cc-login: <原因>`，命令行写错时再加一行 `Run 'cc-login CMD --help' for usage.`，退出码 1。报错不回显参数，以免误粘贴的 token 进终端记录。
- 与 zsh 版逐字节一致的要求只保留给命令的输出、退出码、写的文件和数据目录布局（CLAUDE.md 的移植规则）。
- 补全用 cobra 的 `completion` 命令（zsh、bash、fish、powershell），login 名字由 `ValidArgsFunction` 列出，选项由 cobra 自动补全，替代 zsh 版手写的 `_cc_login_complete`；zsh shim（0004）负责加载。
- 交互界面用 `charmbracelet/bubbletea` + `bubbles` + `lipgloss`，入口是单独的 `cc-login tui`：显示和 `list` 相同的信息，可选中一个 login 执行 use / run / renew / remove。
- `cc-login` 不带参数仍等于 `list`，纯文本输出与 zsh 版一致；TUI 只在显式调用、且 stdin 和 stdout 都是终端时启动，否则报错退出 1。
- 确认提问和不回显输入用 `charmbracelet/huh`，只在终端里用；非终端时与 zsh 版一样从 stdin 读，需要确认的命令提供 `--yes`。
- lipgloss 只用于 TUI；`list` 的颜色继续用现有的 ANSI 码，保证与 zsh 版逐字节一致。

## 后果

- 依赖变多，二进制变大几 MB；启动时间仍在毫秒级。
- 行为与 zsh 版有差异：选项可以出现在任意位置（`add --api-key NAME`），报错文案不同；`cmd/cc-login/main_test.go` 检查退出码、报错原因和 help 提示。
- TUI 没有 zsh 版可比对，靠 bubbletea 的模型单元测试（发送按键消息、检查状态和视图）。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
