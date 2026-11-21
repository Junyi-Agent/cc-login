# 0017 token login 由 cc-login 自己走 OAuth，多要 user:profile

状态：采纳

## 背景

`claude setup-token` 生成的 token 只有 `user:inference` 权限：claude 读不到账号，`list` 的账号列靠用户在 `add` 时手填（0015），claude 里的 `/status` 也没有邮箱和组织。setup-token 不能选 scope，也没有非全屏的输出方式（0013）。

2026-11-22 用户手动试过：用 Claude Code 的 OAuth client 自己发授权请求，scope 写 `user:inference user:profile`，换 token 时带 `expires_in: 31536000`，服务端照给：

- access token 是 `sk-ant-oat01-…`，和 setup-token 的一样，有效一年；同时给了一个约 27 天的 refresh token。
- 换 token 的响应里直接有 `account.email_address` 和 `organization.name`。
- 用这个 token，`/api/oauth/profile` 返回 200，`count_tokens` 返回 200。
- claude 用 `CLAUDE_CODE_OAUTH_TOKEN` 时默认认为 token 只有 `user:inference`，不读 profile；再设 `CLAUDE_CODE_OAUTH_SCOPES="user:inference user:profile"`，claude 会读 profile，把账号和组织写进它的 `oauthAccount`。

参数取自 claude 2.1.330 的二进制（`claude setup-token` 用的同一套）：client_id `9d1c250a-e61b-44d9-88ed-5944d1962f5e`，授权地址 `https://claude.com/cai/oauth/authorize`，手动回调 `https://platform.claude.com/oauth/code/callback`，换 token `https://platform.claude.com/v1/oauth/token`，PKCE S256。这些都没有公开文档。

## 决策

- **token login 的 `add` 和 `renew` 不再运行 `claude setup-token`**，由新包 `internal/oauth` 走授权码 + PKCE：
  1. 生成 code_verifier、state，拼出授权链接，scope 为 `user:inference user:profile`。
  2. 把链接写到 stderr；stdin 是终端时用 `open`（macOS）或 `xdg-open`（其他）打开浏览器，打不开就只靠这行链接。
  3. 用户在浏览器选账号、同意，页面显示 `CODE#STATE`。cc-login 读这一串：终端上显示成 `*`，不是终端时读一行 stdin。state 对不上时报错，不换 token。
  4. 带 `expires_in: 31536000` 换 token。access token 存进 `oauth-token`（0600，和以前一样）；refresh token 不存。
- **账号和过期时间来自换 token 的响应**：`account` 记 `account.email_address`（响应里没有时保留原值），过期时间是现在加响应的 `expires_in`。不再问账号（去掉 `askAccount`）。token 是服务端刚发的，不再用 `count_tokens` 检查（去掉 `CheckToken`）。
- **state.json 的 login 多一个 `scopes`**：服务端实际给的 scope。`run`（以及 `list`、`use` 里跑的 claude）对有 `scopes` 的 token login 设 `CLAUDE_CODE_OAUTH_SCOPES`，claude 因此会读 profile。`CLAUDE_CODE_OAUTH_SCOPES` 加进 `credVars`：shell 里的值不会带进任何 login 的 claude。
- 以前用 setup-token 存的 token login 没有 `scopes`，照旧能用，claude 也照旧读不到账号；`cc-login renew NAME` 一次就换成新流程的 token。
- **不能再粘贴现成的 token**：`add` 读的是授权码。要别处生成的 token，用 sign-in 或 API key。
- `internal/claude` 去掉 `SetupToken`，`app.Claude` 接口少一个方法。Prompter 多一个 `Open(url)`：终端上打开浏览器，否则什么也不做。
- 新设置 `CC_LOGIN_OAUTH_URL`：换 token 的服务器，默认 `https://platform.claude.com`；测试指向本地的假服务器，端到端脚本可以测完整的 `add`。

## 后果

- token login 在 `list` 里有账号，不用手填；claude 里能看到邮箱和组织。
- 依赖 claude 没公开的 OAuth 参数。Anthropic 改了 client 的回调、scope 规则或 `expires_in` 的上限，`add` 和 `renew` 的 token 部分就会失败；要从新版 claude 里重新找参数。sign-in 和 API key 不受影响。
- 服务端如果不给一年（给的 `expires_in` 更短），cc-login 按实际值记过期时间，不报错。
- 用户少了「从别处粘贴一个 setup-token 的 token」的路。
- 0013 里 setup-token 的部分、0015 里「token 另问账号、用 API 检查」的部分由本 ADR 取代。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
