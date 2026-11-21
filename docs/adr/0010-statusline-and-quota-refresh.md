# 0010 cc-login 不做状态栏和定时刷新配额

状态：采纳

## 背景

配额有两条来源，由本机的 Python 脚本完成，不在这个仓库里：

- 状态栏脚本（Claude Code 的 `statusLine` 命令）：从 stdin 读 Claude Code 给的 `rate_limits`，写配额缓存，输出一行配额，在后台把配额作为 OTLP 指标上报。
- 定时刷新（launchd）：给 token login 跑最小的 `claude -p`，取配额交给状态栏脚本写缓存和上报。

两个脚本都按 zsh 版的布局找 login，`cc-login migrate`（0008）之后找不到 login。`cc-login list` 原来在拿不到实时配额时读状态栏脚本写的缓存（`~/.claude/quota-last.json`，`CC_LOGIN_QUOTA_CACHE`），带 `*` 显示。

最初的 0010 打算把这两个功能做成 cc-login 的子命令。用户 2026-11-21 决定不做，并从 Go 里彻底去掉相关部分。

## 决策

- cc-login 不提供状态栏和定时刷新配额的命令，也不上报指标。
- `list` 只显示实时配额（用 sign-in 的 access token 请求 `api/oauth/usage`）；拿不到时显示 `-` 或 `?`，脚注说明原因。不再读配额缓存，`CC_LOGIN_QUOTA_CACHE` 和 JSON 的 `quota_source`、`cached_at` 字段一并去掉。
- 本机的 Python 脚本、launchd 任务和 Claude Code 的 `statusLine` 设置不属于这个仓库，由用户自己处理。

## 后果

- token login 和 API key login 在 `list` 里没有配额（token 读不到，key 没有订阅配额）；sign-in 的 access token 过期时显示 `?`，用这个 login 跑一次 claude 会刷新它。
- 代码里不再有缓存格式和缓存路径，`internal/quota` 只剩配额的类型。
- 两个 Python 脚本在迁移后找不到 login；要继续用，得由用户改成读 `state.json`（或调用 `cc-login list --format json`）。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
