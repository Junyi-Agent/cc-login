# 0003 合并 PR 前要求 CI 检查全部通过

状态：采纳

## 背景

CI 在 Ubuntu 和 macOS 上跑 lint 和测试（macOS 才走 Keychain 代码）。PR 红着也能合并的话，CI 就只是建议。

## 决策

- `.github/workflows/ci.yml` 在 `pull_request` 和推送到 `main` 时跑 `pre-commit run --all-files`，矩阵为 `ubuntu-latest`、`macos-latest`。
- agent 合并 PR 只能用 `gh pr merge`：`git_guard.py` 先跑 `gh pr checks`，没全过就拒绝；直接调合并 API（`gh api .../pulls/N/merge`）也拒绝。
- GitHub 上给 `main` 开分支保护，要求 `ci` 的两个 job 通过才能合并，并禁止直接 push。远端有 `main` 之后由仓库管理员设置。
- 打 `v*` tag 时 `.github/workflows/release.yml` 用 goreleaser 发布 darwin/linux 的 amd64/arm64 二进制。

## 后果

- 分支保护设置之前，人仍能在网页上合并红色 PR 或直接 push `main`。
- release workflow 在第一次打 tag 之前没跑过。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
