# 0014 用 scripts/ship.sh 一步合并 feature 分支

状态：采纳

## 背景

按 0001 和 0003，一个改动进 `main` 要依次：push 分支（跑审查门禁）、开 PR、等 CI、`gh pr merge --squash`、删 worktree 和本地分支、更新本地 `main`。2026-11-22 用户觉得步骤多、要一步步等，提出直接 squash merge 到 `main`。

直接推 `main` 要关掉分支保护的「必须经 PR」和「检查必须通过」：本地 squash 出的提交是新的，没跑过 CI，GitHub 会拒绝推送。那样 Ubuntu 和 macOS 的 CI 就只在提交进 `main` 之后才跑。用户选择保留 PR 和 CI 门禁，把这些步骤做成一个脚本。

## 决策

- `scripts/ship.sh merge branch` 在 feature worktree 里运行（调用形式见 [[0022-keep-command-structure]]），依次：
  1. 检查：在 feature 分支上、没有未提交的改动、包含 `origin/main`（分支保护要求同步）、`gh` 当前账户是仓库所有者、`claude` 已登录（审查门禁要用）；任何一项不满足就在推送前退出。
  2. `git push -u origin <branch>`，pre-push 审查门禁照常跑。
  3. 复用这个分支已开的 PR，没有就新开：标题取最后一个提交的标题，正文列出各个提交。
  4. `gh pr checks --watch --fail-fast` 等 CI，有检查失败就退出。
  5. `gh pr merge --squash`；远端分支由 GitHub 的「合并后删除分支」删除。
  6. 删除 worktree 和本地分支，在主目录 `git pull --ff-only`。`--keep` 保留 worktree 和分支。
- 0001 的 worktree 规则、0002 的审查门禁、0003 的分支保护和 CI 要求都不变。脚本里的 `gh pr merge` 不经过 `git_guard.py`（hook 只看到 `scripts/ship.sh`），等 CI 的检查由脚本第 4 步做，GitHub 的分支保护也会拒绝合并没通过检查的 PR。
- agent 推送前仍要问用户（CLAUDE.md）；用户同意后跑 `scripts/ship.sh` 代替手动的几步。

## 后果

- 合并一个改动只需一条命令，仍要等审查门禁（约 1–3 分钟）和 CI（约 3 分钟）。
- 脚本没有自动测试：它的每一步都调 `gh` 和 GitHub，只能真实运行验证。推送前的检查可以在本地试（未提交改动、在 `main` 上会直接退出）。
- agent 的非交互 shell 没有登录 claude 时，第 1 步就会失败；要在有默认 login 的终端里跑。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
