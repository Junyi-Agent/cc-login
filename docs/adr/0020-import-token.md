# 0020 add --import-token 导入已有的 OAuth token

状态：采纳

## 背景

0017 之后 `add` 只走 cc-login 自己的 OAuth 流程，不能再粘贴现成的 token，理由是 cc-login 发的 token 带 `user:profile`。但用户手里有别处生成的 token（`claude setup-token` 的 `sk-ant-oat01-…`，或别人给的），重新走一遍浏览器授权不总是可行（账号不在手边、token 要交给没有浏览器的机器）。0017 的「用户少了粘贴 token 的路」现在要补回来。

## 决策

- `cc-login add NAME --import-token`：读一个现成的 OAuth token，存成 token login。和 `--sign-in`、`--api-key`、`--oauth-token` 互斥。
- token 从 stdin 读：终端上显示成 `*`，不是终端时读一行 stdin。不放在命令行参数里（`ps` 看得见）。只收 `sk-ant-oat` 开头、不含空白的值。
- 存之前用 `/api/oauth/profile` 检查：
  - 200：有 `user:profile`。像 0018 那样记 `org`、`plan`、`account`，`scopes` 记 `user:inference user:profile`。
  - 401：服务端不认这个 token，报错，什么也不存。
  - 其他（403 = 没有 `user:profile`，如 setup-token 的 token；没有响应、5xx）：照存，不记 `scopes`，stderr 说明；和 0017 之前的 token login 一样，`list` 的额度列是空的。
- 过期时间不知道（token 里没有），不记 `token_expires_at`，`list` 的 `EXPIRES` 为 `-`。`cc-login renew NAME` 之后换成新流程的 token。
- `anthropic.Profile` 和 `Usage` 的错误改成带状态码的 `StatusError`，应用层按状态码区分，不比对字符串。

## 后果

- 别处生成的 token 能直接加进来；没有 `user:profile` 的 token 只能启动 claude，没有账号、组织和额度。
- 导入的 token 不知道过期时间，`doctor` 和 `list` 不会提前提醒。
- 取代 0017 里「不能再粘贴现成的 token」一条。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
