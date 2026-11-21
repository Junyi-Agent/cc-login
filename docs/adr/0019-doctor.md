# 0019 cc-login doctor 检查配置

状态：采纳

## 背景

最常见的配置错误不报错：没装 `claude` 链接时，`cc-login use` 照常成功，但 `claude` 直接跑真正的 claude，默认 login 不起作用（2026-11-22 测试时遇到）。`version` 会说链接有没有装，但要用户自己去读；shell 里残留的 `ANTHROPIC_API_KEY`、权限太松的凭据文件、过期的 token 也只有在出问题之后才被发现。

## 决策

- 新命令 `cc-login doctor`，只做离线检查，不联网，不调 `claude auth status`：
  - claude 能找到；`claude` 链接装了，并且在 PATH 里排第一。
  - shell 里设了 `ANTHROPIC_API_KEY` 或 `CLAUDE_CODE_OAUTH_TOKEN`（cc-login 的 claude 不带它们，没有链接的 claude 会用）。
  - state.json 能读；有默认 login。
  - 每个 login：凭据目录在、权限不让别人读（目录 0700，key 或 token 文件 0600）；key 或 token 文件在；token 没过期、一周内不过期；token 有 `user:profile`。
  - `~/.claude/.credentials.json` 里有没被 cc-login 管的 sign-in（提示 `adopt`）。
- 每行以 `✓`、`!`、`✗` 开头（正常、警告、问题），不正常的下面一行写 `fix:`，给出要运行的命令。
- 有 `✗` 时退出码 1，只有警告时 0。输出在 stdout，错误汇总在 stderr。
- 不读 key 或 token 的内容，只看文件在不在、谁能读；输出里没有任何凭据。
- sign-in login 的过期不查（要调 claude），仍由 `list` 的 STATUS 和 EXPIRES 列显示。

## 后果

- `version` 的链接检查和 `doctor` 重复，保留：`version` 短，常用来看版本。
- 以后新增的配置要求，在 `internal/app/doctor.go` 里加一条检查。
