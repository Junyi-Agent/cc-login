# 0011 精简：去掉 migrate、token 叠加、多余的输出格式

状态：采纳

## 背景

2026-11-21 审视代码时，复杂度集中在四处：`list` 的配额、token 登录的特例、`list` 的输出格式和表格排版、一次性的 `migrate`。用户决定精简这四处，并修掉小处的重复。

## 决策

- **去掉 `migrate`**：删除 `cc-login migrate`、`internal/state/migrate.go`，以及没有 state.json 时对旧布局的检测（`ErrLegacy`）。还在用 zsh 版的机器用提交 `6a3cea9` 的版本迁移一次（README 写了命令）。0008 里关于 `migrate` 的部分随之失效。
- **配额保留，写法精简**：只给 sign-in login 读实时配额。`View.Quota` 为 nil 表示没有读到，`View.QuotaErr` 说明读失败的原因（access token 过期、请求失败），表格里显示 `?` 并在脚注说明；token 和 API key login 不读，显示 `-`。去掉 `quota` 包和 5 个值的 `QuotaGap`，`anthropic.Usage` 直接把 JSON 解码进返回的结构。
- **token 另建 login**：`add NAME` 遇到已有的 NAME 一律报错，提示用 `renew`，或先 `remove`。不再把 token 叠加在 sign-in login 上，也不再复用已登出 login 的目录。已经叠加过的 login 仍按 token login 使用，只是不再显示配额。
- **输出格式只留 JSON**：`list --json` 代替 `--format json|tsv|TEMPLATE`。表格不再用 `lipgloss/table`，自己按列宽补空格；终端太窄时只去掉日期列，不再截断账号。

## 后果

- 删掉约 700 行代码和测试，`internal/quota` 包、`cc-login migrate` 命令、`--format` 选项不复存在。
- 依赖 `--format tsv` 或模板的脚本要改用 `--json` 加 `jq`。
- 账号很长、终端很窄时表格会折行。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
