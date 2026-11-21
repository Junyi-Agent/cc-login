# 0007 claude 由 cc-login 用 exec 启动，不再往 shell 导出凭据

状态：采纳

## 背景

zsh 版在每条命令前（preexec）把默认 login 的 `CLAUDE_SECURESTORAGE_CONFIG_DIR`，以及 `ANTHROPIC_API_KEY` 或 `CLAUDE_CODE_OAUTH_TOKEN` 导出到交互 shell。于是这个 shell 启动的每个进程都拿得到 key：构建脚本、`npm install` 的 postinstall、任何 CLI。0004 打算在 Go 版保留这个 shim。用户 2026-11-21 决定换掉这个做法。

## 决策

- `cc-login run [NAME] [-- ARGS...]`：不给 NAME 时，用外层 claude 的 login（cc-login 启动的 claude 在环境里带 `CC_LOGIN_ID`，它再启动的 claude，比如 `claude -p`、hook，沿用同一个 login），没有外层时用默认 login。cc-login 去掉环境里所有 login 相关的变量，只加这个 login 的目录和 key 或 token，然后用 `syscall.Exec` 换成真正的 claude。key 和 token 只出现在这一个 claude 进程的环境里，不进 argv、不进 shell。
- 多入口二进制：cc-login 被叫作 `claude` 时（`argv[0]` 的文件名是 `claude`，例如 `~/.local/bin/claude -> cc-login` 的链接），等于 `cc-login run -- ARGS...`。把这个链接放在 PATH 里真正的 claude 之前，IDE、脚本、审查门禁等任何按 PATH 找 claude 的程序都会用默认 login。
- 找真正的 claude：按 PATH 顺序找名为 `claude` 的可执行文件，跳过解析后与 cc-login 自身是同一个文件的项（`os.SameFile`）。环境变量 `CC_LOGIN_CLAUDE` 可直接指定路径。再设一个 `CC_LOGIN_DEPTH` 计数，每经过一次 cc-login 加一，到 10 层就报错，防止链接配错时无限循环；claude 里再启动 claude 是正常的嵌套，所以上限不是 1。
- 不再需要 zsh shim；补全用 `cc-login completion zsh|bash|fish`。0004 由本决策取代。

## 后果

- 打开的终端不用重新加载什么：每次启动 claude 都读当时的默认 login。
- 用绝对路径直接调用真正 claude 的程序绕过 cc-login，拿不到任何 login；这时 claude 用它自己的默认配置目录。
- bash、fish 等 shell 不用各写一份 shim。
- shell 里不再有 `ANTHROPIC_API_KEY`；以前依赖这个变量的其他工具要自己设。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
