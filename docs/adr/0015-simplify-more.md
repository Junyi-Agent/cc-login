# 0015 再精简：token 的组织推断、renew 的额外检查、表格和版本号

状态：采纳

## 背景

0011 之后用户 2026-11-21 再审视一次代码，要求把剩下收益小的部分全部清理。复杂度来自几处：token login 为了显示 plan 推断组织；`add` 读 key 和 token 的两段代码重复；`renew` 为了多报一句话跑 3 次 `claude auth status`；表格有两列过期时间和一个主题设置；版本号手动拼 commit。

这些改动最初写在分支 `simplify-2`（ADR 编号 0012），没有合并；之后 0012（代码布局）和 0013（上手）先进了 `main`，2026-11-22 移植到新的代码布局上，改编号为 0015。

## 决策

- **token 不再推断组织**：`CheckToken` 只返回结论和 HTTP 状态，`anthropic-organization-id` 头只在它内部用来判断 token 已通过认证（模型退役返回 404 时仍然算通过）。去掉 `tokenOrg`（从同组织的其他 login 借组织名和 plan），state.json 去掉 `org_id`、`org_name`、`token_saved_at`，`claude.AuthInfo` 不再读 `orgId`、`orgName`。旧的 state.json 里这几个字段读时忽略，下次写入时消失。token login 的 plan 只有用户原来存的值，通常显示 `-`。
- **key 和 token 共用一段代码**：`saveSecret` 按 kind 取提示文案、前缀和检查函数；API key 另记末 4 位，token 先跑 `claude setup-token`（0013），再另问账号、记一年后的过期时间。仍然用 API 检查，报错文案不变。
- **renew 只做登录、保存、报告新的过期时间**：去掉“过期时间没往后移”的报错、失败后判断旧登录是否还在、组织变化的提示。失败时直接返回 `claude auth login` 的错误，state.json 不变。
- **表格的过期时间只留一列相对时间**（`in 30d`、`5h ago`、`never`），准确时间看 `list --json`。终端宽度不再参与排版；去掉 `CC_LOGIN_THEME`，默认 login 的高亮用一个固定的 256 色（208）。
- **版本号直接用 `debug.ReadBuildInfo().Main.Version`**：goreleaser 设置的 `main.version` 优先；本地 `go build` 时 Go 记的是带 commit 的伪版本（`v0.0.0-时间-commit`，改过的工作区带 `+dirty`），不再自己解析 `vcs.revision`。0013 加的 claude 和链接两行不变。
- **改状态的命令把确认写到 stderr**：`use`、`rename`、`remove`、`add`、`renew`、`adopt` 成功时说明改了什么、下一步做什么，这些都写 stderr；stdout 只有 `list` 和 `version` 的结果，放进管道时是干净的。
- `credVars` 去掉 zsh 版留下、没人再设置的 `CC_LOGIN_API_KEY_DIR`、`CC_LOGIN_OAUTH_TOKEN_DIR`。
- **保留** `CC_LOGIN_DEPTH`（0007）：`CC_LOGIN_CLAUDE` 不经过 `os.SameFile` 检查，用 `cp` 复制成 `claude` 的 cc-login 也绕得过它，没有计数会无限 exec。
- 审查门禁的设计审查提示词不再问 `migrate`（0011 已去掉）和 zsh 布局的链接。

## 后果

- `list --json` 的字段不变；直接读 state.json 里 `org_id`、`org_name`、`token_saved_at` 的外部脚本要改。
- token login 不再显示从别的 login 借来的 plan。
- renew 失败时不再说明旧的登录是否还在；需要时运行 `cc-login list` 查看。
- 终端很窄时表格折行（0011 已有此后果，这里少了一列，折行更少）。
- 0009 里 `CC_LOGIN_THEME` 的部分失效。
- 解析这些命令 stdout 的脚本读不到确认信息了；它们应该看退出码。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
