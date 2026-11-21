# 0010 状态栏和定时刷新配额改由 cc-login 做

状态：提议

## 背景

配额有两条来源，现在都由本机的 Python 脚本完成，不在这个仓库里：

- 状态栏脚本（Claude Code 的 `statusLine` 命令）：从 stdin 读 Claude Code 给的 `rate_limits`，写配额缓存（按 `accountUuid/orgUuid` 和 `dir:ID` 两个键，`dir:ID` 带写入时间），输出一行 `Claude | 5h: 30%  7d: 50%`（没有新数据时用缓存，带 `*`），再在后台进程里把配额作为 OTLP 指标上报（带账号、org 等属性），收集端在本机或 `OTEL_EXPORTER_OTLP_*` 指定的地址。
- 定时刷新（launchd，每 5 分钟）：给 token login（以及显式开启的 sign-in login）各跑一次最小的 `claude -p`，从 stream-json 的 `rate_limit_event` 取配额，交给状态栏脚本写缓存和上报。默认不探测 sign-in login：跑一次 claude 可能刷新它的 token，刷新会让别处同一份 sign-in 失效。

两个脚本都按 zsh 版的布局找 login（`NAME -> .dirs/ID` 链接、`NAME.json` 记录）。`cc-login migrate`（0008）之后链接和记录移走，定时刷新找不到任何 login，状态栏读不到 token login 的账号。

0010 最初写成“statusline 只读缓存、不发网络请求”，与上面的事实不符，已改回「提议」，待用户决定。

## 可选方案

1. 全部移进 Go：`cc-login statusline`（读 stdin、写缓存、输出一行、上报 OTLP）和 `cc-login quota-refresh`（探测、写缓存、上报）。OTLP 用 `go.opentelemetry.io/otel` 的 SDK 和 otlpmetrichttp 导出器，依赖增加较多；指标名、属性、`service.instance.id` 要与现在一致，否则仪表盘断档。
2. Go 只接管找 login 的部分：给 `list --format` 加上 ID 和凭据目录字段，Python 脚本改为调用 `cc-login list --format json` 找 login、读账号；缓存和 OTLP 留在 Python。
3. 先不动：迁移前不运行 `cc-login migrate`，或迁移后让 Python 脚本读 `.legacy-<时间>/` 里的旧链接。

## 决策

（待用户选择。）

## 后果

（随决策填写。）

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
