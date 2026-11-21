# 0013 上手：add 自己跑 setup-token，报错指出下一步，version 检查链接

状态：采纳（运行 setup-token 的部分已被 [[0017-own-oauth]] 取代）

## 背景

2026-11-21 从用户角度审视用法：日常只用 `claude`、`cc-login`、`cc-login use`，麻烦集中在第一次上手。

- 默认的 token login 要用户另开终端跑 `claude setup-token`，再回来粘贴。
- `cc-login run -p hi` 报 `unknown shorthand flag: 'p'`，不说 claude 的参数要放在 `--` 后面；`add --sign-in`、`renew` 一样。
- 一个 login 都没有时，`run` 和 `claude` 链接报「没有默认 login，用 cc-login use」，这时没有可 use 的 login。
- `claude` 链接要手动装，README 同时给链接和 alias 两条路，装完没办法确认 PATH 里的 claude 是不是 cc-login。

自动截获 setup-token 打印的 token 也考虑过：claude 2.1.330 的 setup-token 是 Ink 全屏界面，没有只输出 token 的选项，截获要引入 pty 依赖并从屏幕输出里抠 token，换行、颜色码和 claude 界面改版都会让它失效。用户选了不截获。

## 决策

- `add`（token，默认）和 token login 的 `renew`：stdin 是终端时，cc-login 在同一个终端运行 `claude setup-token`（stdin/stdout/stderr 直通，环境只有新 login 的目录、没有任何 key 或 token），它退出后再弹出粘贴框。Ctrl-C 中止且不保存；claude 运行失败时提示到别的终端运行，仍可粘贴。stdin 不是终端时不运行 setup-token，照旧读一行 stdin。
- `run`、`add`、`renew` 遇到未知选项时，报错补一句：claude（或 `claude auth login`）的选项放在 `--` 后面，并给出用法。用 cobra 的 `SetFlagErrorFunc`。
- 没有任何 login 时，`run`（以及 `claude` 链接）报 `no logins yet; add one with: cc-login add NAME`。
- `internal/claude` 多两个函数：`SetupToken`（运行 `claude setup-token`）和 `InPath`（PATH 里第一个 claude，以及它是不是 cc-login 自己），通过 `app.Claude` 接口注入，测试用假的。[[0012-code-layout]] 里 `internal/claude` 的职责相应加上 setup-token。
- `cc-login version` 多打两行：cc-login 运行的 claude，以及 PATH 里第一个 claude 是不是 cc-login（链接是否生效）。README 只推荐链接一种方式，去掉 alias，并用 `cc-login version` 检查。

## 后果

- token login 少开一个终端，仍要粘贴一次。
- setup-token 直接读终端 stdin（raw 模式）。浏览器回调失败、要手动粘贴授权码时，可能遇到 `claude auth login` 那种粘贴被拆开的问题（`internal/claude.AuthLogin` 用管道转发规避）；setup-token 是 Ink 界面，管道 stdin 可能让它进不了 raw 模式，所以没有照搬，待人类实测。
- 测试里 stdin 不是终端，端到端脚本测不到 setup-token 这一步，由 `internal/app` 的单元测试覆盖。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
