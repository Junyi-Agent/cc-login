# 0009 代码结构：app 层、类型、lipgloss 表格、testscript

状态：采纳

## 背景

`cmd/cc-login` 里同时有命令定义和业务逻辑；`Login` 用两个 bool 表示类型、用字符串表示状态、用 int64 秒表示时间；`list` 的表格手写列宽和 ANSI 码（为了与 zsh 版逐字节一致，这个要求已经取消）；每个 login 一个不限数量的 goroutine，Ctrl-C 取消不了；测试用长字符串内联期望输出。后面还要加 TUI，它和命令行做同样的事。用户 2026-11-21 同意按下面重构。

## 决策

- 分层：
  - `internal/state`：state.json 的读写、锁、迁移（0008）。
  - `internal/claude`：找 claude、拼环境、`auth status` / `auth login` / `setup-token` / `auth logout`。
  - `internal/anthropic`：API 调用（检查 key 和 token、配额）。
  - `internal/quota`：配额的类型（0010 之后不再有缓存）。
  - `internal/app`：每个命令一个函数（`List`、`Use`、`Add`、`Run`、`Rename`、`Remove`…），参数进、结果或带类型的错误出（`ErrNotSetUp`、`ErrNoLogin`…），不写 stdout、不依赖 cobra。外部调用（claude、HTTP、钥匙串读取、时钟）通过结构体字段注入。
  - `internal/ui`：表格、JSON、提问；`cmd/cc-login` 只做 cobra 和输出。TUI 也调用 `internal/app`。
- 类型：login 类型用 `Kind` 枚举，状态用 `Status` 枚举，时间用 `time.Time`；表格和 JSON 从同一个视图模型生成。
- 表格用 `charmbracelet/lipgloss` 的 `table`；颜色由 lipgloss/termenv 判断（`NO_COLOR`、非终端、深浅背景），`CC_LOGIN_THEME` 可覆盖。
- 并发：`errgroup.SetLimit` 限并发，每个 login 单独超时；`signal.NotifyContext` 让 Ctrl-C 取消正在跑的子进程和请求。
- 测试：命令行端到端用 `rogpeppe/go-internal/testscript`（`testdata/script/*.txtar`），claude 和 API 用测试里的假程序和 `httptest` 代替；表格输出用 `testdata/*.golden`，`go test -update` 重新生成。
- 设置只来自环境变量（`CC_LOGIN_ROOT`、`CC_LOGIN_THEME`、`CC_LOGIN_CLAUDE`、`NO_COLOR`），取代 0005。
- 调试日志用 `log/slog` 写 stderr，`--verbose` 或 `CC_LOGIN_DEBUG=1` 打开；日志不记 key、token 和它们所在的环境变量。
- `cc-login version` 从 `debug.ReadBuildInfo` 取版本；`list --format json|tsv|<Go 模板>` 给脚本用（`--json` 保留为 `--format json` 的简写）。

## 后果

- 原有的 `internal/store`、`internal/listing` 被 `internal/state`、`internal/app`、`internal/ui` 取代。
- 依赖增加 lipgloss、x/sync、go-internal，二进制再大一些。
- 不再有 zsh 版可比对，行为由 testscript 和 golden 文件锁住。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
