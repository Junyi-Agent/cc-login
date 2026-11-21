# ADR（架构决策记录）

每个影响流程或架构的决策写一篇 `NNNN-<slug>.md`，从 [`template.md`](template.md) 复制。

## 状态

| 状态 | 含义 | 谁来改 |
|---|---|---|
| 提议 | 只写了 ADR 文档 | 写 ADR 的人或 agent |
| 采纳 | 允许去实现，可能已经实现 | 用户决定后，人或 agent 改 |
| 已验证 | 人类测试了代码，确认了实现的内容 | 只能由人类改，并填写「验证」一节 |

- agent 不能把状态改成「已验证」，也不能代填「验证」一节。实现完成后停在「采纳」，请用户测试。
- 被取代的 ADR 状态写「已取代（见 NNNN）」，不删除。

## 列表

| # | 决策 | 状态 |
|---|---|---|
| [0001](0001-worktree-and-git-guard.md) | 在仓库外的 worktree 开发，用 PreToolUse hook 强制 | 采纳 |
| [0002](0002-pre-push-review-gate.md) | push 前 3 个 Claude 审查，≥2 同意才放行 | 采纳 |
| [0003](0003-merge-requires-green-ci.md) | 合并 PR 前要求 CI 检查全部通过 | 采纳 |
| [0004](0004-zsh-shim-exports-env.md) | 保留一个 zsh shim，在 preexec 导出默认 login 的环境变量 | 已取代（见 0007） |
| [0005](0005-zstyle-to-env.md) | zstyle 设置改由环境变量传给 Go | 已取代（见 0009） |
| [0006](0006-cobra-and-bubbletea.md) | 命令行解析用 cobra，交互界面用 bubbletea | 采纳 |
| [0007](0007-launch-claude-by-exec.md) | claude 由 cc-login 用 exec 启动，不再往 shell 导出凭据 | 采纳 |
| [0008](0008-state-json-layout.md) | 数据布局：一个 state.json 加按 ID 存放的凭据目录 | 采纳 |
| [0009](0009-code-structure.md) | 代码结构：app 层、类型、lipgloss 表格、testscript | 已取代（见 0012） |
| [0010](0010-statusline-and-quota-refresh.md) | cc-login 不做状态栏和定时刷新配额 | 采纳 |
| [0011](0011-simplify.md) | 精简：去掉 migrate、token 叠加、多余的输出格式 | 采纳 |
| [0012](0012-code-layout.md) | 代码布局：按命令分文件、统一命名、creds 管凭据目录 | 采纳 |
| [0013](0013-first-run.md) | 上手：add 自己跑 setup-token，报错指出下一步，version 检查链接 | 采纳（setup-token 部分已被 0017 取代） |
| [0014](0014-ship-script.md) | 用 scripts/ship.sh 一步合并 feature 分支 | 采纳 |
| [0015](0015-simplify-more.md) | 再精简：token 不推断组织、renew 只做登录、表格一列过期时间、确认写 stderr | 采纳 |
| [0016](0016-release-binaries.md) | 用 GitHub release 的二进制安装：固定的包名和 install.sh | 已验证 |
| [0017](0017-own-oauth.md) | token login 由 cc-login 自己走 OAuth，多要 user:profile | 采纳 |
| [0018](0018-list-org-and-token-quota.md) | 无命令时给帮助；list 加组织列；token login 读配额和 profile | 已验证 |
| [0019](0019-doctor.md) | cc-login doctor 检查配置：链接、shell 变量、login 文件和过期 | 已验证 |
