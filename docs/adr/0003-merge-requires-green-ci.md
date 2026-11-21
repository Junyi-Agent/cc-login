# 0003 合并 PR 前要求 CI 检查全部通过

状态：采纳

## 背景

CI 在 Ubuntu 和 macOS 上跑 lint 和测试（macOS 才走 Keychain 代码）。PR 红着也能合并的话，CI 就只是建议。

## 决策

- `.github/workflows/ci.yml` 在 `pull_request` 和推送到 `main` 时跑 `pre-commit run --all-files`，矩阵为 `ubuntu-latest`、`macos-latest`。
- agent 合并 PR 只能用 `gh pr merge`：`git_guard.py` 先跑 `gh pr checks`，没全过就拒绝；直接调合并 API（`gh api .../pulls/N/merge`）也拒绝。
- GitHub 上给 `main` 开分支保护：要求 `pre-commit (ubuntu-latest)`、`pre-commit (macos-latest)` 两个检查通过、分支与 `main` 同步（strict），必须经 PR 合并，线性历史，禁止 force push 和删除，管理员也受限制（enforce admins）。仓库只允许 squash 合并，合并后删除分支。2026-11-21 已设置。
- 打 `v*` tag 时 `.github/workflows/release.yml` 用 goreleaser 发布 darwin/linux 的 amd64/arm64 二进制。

## 后果

- 人也不能直接 push `main` 或合并红色 PR；紧急情况要先在仓库设置里关掉保护。
- release workflow 在第一次打 tag 之前没跑过。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
