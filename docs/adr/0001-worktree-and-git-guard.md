# 0001 在仓库外的 worktree 开发，用 PreToolUse hook 强制

状态：采纳

## 背景

agent 在主目录直接切分支或在 `main` 上提交，会打乱用户正在用的工作区，也会绕过 PR 和 CI。只写在 CLAUDE.md 里，agent 可能不遵守。

## 决策

- 每个新功能在仓库目录之外建 worktree：`git worktree add ../go-cc-login-<feature> -b <feature> origin/main`。
- `.claude/hooks/git_guard.py`（PreToolUse，Bash，取自 `Junyi-Agent/skills`）拒绝：在当前目录 `git switch` / `git checkout <branch>`、在 `main` 上产生提交（commit、merge、cherry-pick、revert、am）、push 到 `main`、用 `--no-verify` / `SKIP=` / `core.hooksPath` 跳过 git hook、CI 没全过时 `gh pr merge`。
- 分支按命令实际运行的目录判断，跟随 `cd`、`pushd`、子 shell、`env -C`、`git -C`，透过 `env`、`bash -c` 等包装和 git 别名解析。hook 出错或解析不了提到 git/gh 的命令时拒绝（fail closed）。
- 远端还没有 `main` 时，第一次提交和推送由人类在终端完成。

## 后果

- 拦截只对 Claude Code 的 Bash 工具生效；人在终端里操作不受限。
- `export SKIP=...` 后单独 push、`eval`、脚本文件里的 git 命令拦不住，靠 CLAUDE.md 约束；`git push --all` 带上 main 由服务端分支保护拦。
- hook 出错会拦下所有 Bash 命令，修 hook 时先跑 `.claude/hooks/test_git_guard.py`。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
