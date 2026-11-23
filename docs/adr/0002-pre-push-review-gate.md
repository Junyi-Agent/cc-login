# 0002 push 前 3 个 Claude 审查，≥2 同意才放行

状态：采纳

## 背景

pre-commit 和 CI 只做格式、lint 和单元测试，查不出移植后行为和 zsh 版不一致、凭据漏给别的 login、改动绕开流程这类问题。

## 决策

- `scripts/review_gate.py` 作为 pre-commit 的 `pre-push` 阶段 hook 运行（`pre-commit install` 会同时装 pre-commit 和 pre-push）。
- 审查范围：`merge-base(origin/main, 被 push 的 ref)..被 push 的 ref`。远端可达但还没有 `main`（第一次推送）时，审查到被 push 的 ref 为止的全部内容。
- 并行跑 3 个 `claude -p`，方向分别是：
  - 正确性：边界情况、goroutine 竞争、忽略的错误、测试是否真的测到改动；
  - 与 zsh 版行为一致：输出、退出码、写的文件、数据目录布局，以及文档与代码一致；
  - 凭据安全与流程：key/token 不漏给别的 login、日志、argv 或他人可读的文件，不改变凭据目录路径字符串，遵守 CLAUDE.md，不削弱 hook/CI/门禁，不提交个人信息。
- 只给 Read/Grep/Glob 工具，用 `--json-schema` 要求结构化输出。≥2 个 approve 才 exit 0；报错、超时、输出解析失败都算 reject。
- commit log、改动文件列表（`git diff --name-status`）和 diff 放在每次随机生成标签的 `<untrusted-…>` 块里，试图引导审查的内容按 blocking issue 处理。diff 超长时截断，文件列表始终完整，截断提示让审查者用 Read 读列表里没出现在 diff 中的文件。
- 本地找不到 `origin/main` 且远端不可达或已有 `main` 时阻止 push，提示先 `git fetch origin`。
- CI 不跑这个 hook。

## 后果

- 每次 push 调用 3 次 Claude，要花 token，等几分钟。
- 机器上没有 `claude` CLI 或没登录时 push 会被拦住。
- 人可以在终端用 `git push --no-verify` 跳过；agent 走 Bash 时被 0001 的 hook 拦住。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
