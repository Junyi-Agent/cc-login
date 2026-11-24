# cc-login (Go)

一台机器上管理多个 Claude Code 登录，claude 由 cc-login 用 exec 启动。最初是 `cc-login.zsh` 的 Go 移植，现在独立设计，不再与 zsh 版兼容（2026-11-21 用户决定）。上手见 [[README]]，用法见 [[docs/usage]]，开发和进度见 [[docs/development]]，设计见 `docs/adr/`（0006–0022）。

## 开发流程

- 每个新功能或修改在仓库目录之外建 worktree 开发：
  `git worktree add ../go-cc-login-<feature> -b <feature> origin/main`
- 不在当前目录切换分支（`git checkout <branch>` / `git switch`）。
- 不在 `main` 上提交，不 push 到 `main`。push feature 分支后开 PR，CI 通过后用 `gh pr merge --squash` 合并。
- 合并用 `scripts/ship.sh merge branch` 一步做完：检查 → push（跑审查门禁）→ 开 PR → 等 CI → squash merge → 删 worktree 和分支、更新本地 `main`。见 [[docs/adr/0014-ship-script]]。
- 不用 `--no-verify`、`SKIP=`、`core.hooksPath` 跳过 git hook。
- 以上规则由 `.claude/hooks/git_guard.py`（PreToolUse hook）强制执行；`gh pr merge` 会先跑 `gh pr checks`，检查没全过就拒绝。见 [[docs/adr/0001-worktree-and-git-guard]]、[[docs/adr/0003-merge-requires-green-ci]]。
- 推送前问用户；推送用 `gh` 的当前账户，先用 `gh auth status` 确认是 `Junyi-Agent`。
- 远端还没有 `main` 时，第一次提交和 `git push -u origin main` 由人类在终端完成（git guard 会拦 agent）；审查门禁在这种情况下审查全部内容。

## push 审查门禁

push 时 pre-push hook 跑 `scripts/review_gate.py`：3 个 Claude 分别从正确性、是否符合 ADR 和文档、凭据安全与流程审查 `origin/main` 之后的改动，≥2 个同意才放行。被拒时按输出里的 blocking issues 改完再 push。需要本机装好并登录 `claude` CLI：门禁按 PATH 调用 `claude`，所以默认 login 要能用（装了 `claude -> cc-login` 链接之后自动满足；没装时，agent 的非交互 shell 没有登录环境，要在带默认 login 环境的进程里 push）。见 [[docs/adr/0002-pre-push-review-gate]]。

## 本地检查

新机器或新 worktree 先装工具和 hook（同时装 pre-commit 和 pre-push 两个阶段）：

```bash
brew install go pre-commit   # 或 pipx install pre-commit
pre-commit install
```

提交前会跑：YAML/JSON 校验、`golangci-lint fmt`（gofmt + goimports）、`golangci-lint run`（规则见 `.golangci.yml`）、`go mod tidy -diff`、`go test -race ./...`，改到对应文件时还跑 git guard 和审查门禁的单元测试。手动全量跑：`pre-commit run --all-files`。CI（`.github/workflows/ci.yml`）在 Ubuntu 和 macOS 上跑同一套配置，另外跑 `govulncheck`：代码用到的 Go 标准库或依赖有已知漏洞时失败，按它说的升级 `go.mod` 里的 Go 版本或依赖。

## 设计规则

- 数据布局以 [[docs/adr/0008-state-json-layout]] 为准：元数据只在 `state.json`，写之前加锁、原子替换；不移动、不改名凭据目录（Keychain 条目名是目录路径的 SHA-256 前 8 位，路径一变 login 就未登录）。
- claude 只经 `cc-login run` 或 `claude` 链接用 exec 启动（[[docs/adr/0007-launch-claude-by-exec]]），不往 shell 导出凭据。
- 分层和文件布局见 [[docs/adr/0012-code-layout]]：业务逻辑在 `internal/app`，不写 stdout、不依赖 cobra；`cmd/cc-login` 只做命令行。每个命令在 `cmd/cc-login/<命令>.go` 和 `internal/app/<命令>.go` 各一个文件；凭据目录里的东西（key/token 文件、access token、claude 的环境）只在 `internal/creds`。
- help、用法和参数报错由 cobra 生成（[[docs/adr/0006-cobra-and-bubbletea]]）。选项注册给 cobra，参数检查用 cobra 的 `Args`，不手写解析。
- 设置只来自环境变量（`docs/usage.md` 的设置表），新增时写进那张表。
- README 只放一句介绍和 Getting Started（安装、加 login、装 `claude` 链接、日常三条命令）；其余内容放 `docs/`。

## 安全

- 一个 login 的 API key 或 token 只能出现在这个 login 的 claude 进程环境里：不进其他 login 的进程、不进日志和错误信息、不进命令行参数（`ps` 看得见），文件权限 0600。
- 仓库是公开的：不提交本机路径、邮箱、账号名、凭据。测试数据用 `example.com` 之类的假值。

## 约定

- 流程或架构决策写 ADR（`docs/adr/`）。状态分三种：提议 = 只写了文档；采纳 = 允许实现，可能已实现；已验证 = 人类测试了代码并确认。agent 最多改到「采纳」，「已验证」只由人类改。
- 测试不碰真实凭据和网络：外部调用（claude、Keychain、HTTP）都通过可替换的函数注入。
- 代码按块写：函数里的每个功能块（准备、每个前置检查、每一步、返回）之间空一行，读代码时一块一块读，而不是一行一行读。一个函数做了几件不相关的事或嵌套太深时，拆出带注释的小函数。测试里的用例表单独写成 `cases` 变量，和循环之间空一行。

## 已知的故意设计

审查（`/jy-repo-review`）不把下列行为当问题上报。

- `.golangci.yml` 对 `internal/claude`、`internal/creds` 豁免 G204：运行 `claude` 和 `security` 的参数由 cc-login 自己拼，不经过 shell。
- `CC_LOGIN_API_URL`、`CC_LOGIN_OAUTH_URL` 是给测试用的设置：testscript 把它们指向本地不存在的地址，保证测试不连网。
- `internal/oauth` 用 Claude Code 的 OAuth client_id 和没公开的地址（[[docs/adr/0017-own-oauth]]）；`cmd/cc-login/prompt.go` 用 `open`/`xdg-open` 打开 cc-login 自己拼的登录链接（G204 用 nolint 注明）。
- `git_guard.py` 和 `review_gate.py` 来自 `Junyi-Agent/skills`，`review_gate.py` 改了审查方向，并在远端为空时审查全部内容。
- 不考虑并发（2026-11-23 用户决定）：cc-login 按一次只有一个人在一台机器上用来设计。同时跑两个改 state.json 的命令可能互相覆盖，比如并发 `add` 写出两个同名 login、`renew` 结束时撤销期间的 rename，这些不当问题上报。
- `export SKIP=...` 之后单独 `git push` 拦不住：git guard 只看命令前面的 `VAR=x`，见 [[docs/adr/0001-worktree-and-git-guard]]，靠本文件的规则约束。
- cc-login 的子命令不是 kubectl 式结构、`list` 用 `--json` 不用 `-o`：[[docs/adr/0022-keep-command-structure]] 豁免了。
