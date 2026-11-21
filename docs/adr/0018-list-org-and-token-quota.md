# 0018 无命令时给帮助；list 加组织列；token login 读配额和 profile

状态：已验证

## 背景

`cc-login` 不带命令时直接列账号，要探测每个 login，慢，还让人看不到命令。0017 之后 token login 带 `user:profile`，同一个 token 能读 `/api/oauth/usage` 和 `/api/oauth/profile`，但 `list` 还没用上：token login 的配额列是空的，plan 和组织没有记录。

## 决策

- `cc-login` 不带命令输出帮助，`cc-login list` 才列 login。
- `list` 的 `ACCOUNT` 列改名 `EMAIL`，新增 `ORG` 列（在 `EMAIL` 和 `PLAN` 之间）。`list --json` 多一个 `org`；`account` 键不改。
- 有 `user:profile` 的 token login：`list` 用 token 文件里的 token 调 `/api/oauth/usage`，填 `5H LEFT` 和 `WEEK LEFT`。没有这个 scope 的旧 token 照旧不读配额。
- token login 的 `add` 和 `renew` 在换到 token 之后再调一次 `/api/oauth/profile`：`organization.name` 存进 state.json 的 `org`；`organization_type` 去掉 `claude_` 前缀得到 plan（`claude_team` → `team`，`claude_max` → `max`），存进 `plan`；响应里有邮箱时更新 `account`。读 profile 失败不算 `add` 失败：login 照存，stderr 提示，再 `renew` 一次重试。
- sign-in login 的 plan 仍来自 `claude auth status`，没有组织名，`ORG` 显示 `-`。

## 后果

- 依赖没公开的 `/api/oauth/profile`；它变了只会让 `org` 和 `plan` 为空，不影响登录和启动 claude。
- 以前存的 token login 没有 `org`，`renew` 一次才有。
- 读 token 文件里的 token 属于 `internal/creds`；`list` 把它只发给 Anthropic 的 usage 地址，不写日志和错误信息。

## 验证

验证人：Junyi
日期：2026-11-22
测了什么：cc-login 不带命令输出帮助；list 表头是 EMAIL、ORG；用真实账号 add 新的 token login，list 里有邮箱、组织、plan 和 5H/WEEK 额度；renew 旧 token login 后同样有；旧 login 的 ORG 显示 -。
结果：通过。未测：读 profile 失败时 login 照存（只有单元测试覆盖）。
