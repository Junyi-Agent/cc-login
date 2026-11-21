# 0004 保留一个 zsh shim，在 preexec 导出默认 login 的环境变量

状态：已取代（见 0007）

## 背景

zsh 版在每条命令前（`add-zsh-hook preexec _cc_login_sync`）把默认 login 的 `CLAUDE_SECURESTORAGE_CONFIG_DIR`，以及它的 `ANTHROPIC_API_KEY` 或 `CLAUDE_CODE_OAUTH_TOKEN` 导出到当前 shell，于是 `cc-login use` 之后，任何打开的终端下一次启动 claude 都用新的默认 login。Go 程序是子进程，改不了父 shell 的环境。

## 决策

- 保留一个约 30 行的 zsh 文件（`shell/cc-login.zsh`）：在 preexec 时读 `ROOT/current`、解析 `NAME -> .dirs/ID` 链接、导出或清除上面的变量，只用 zsh 内建操作，不 fork。
- 不采用每条命令前 `eval "$(cc-login env)"`：每条命令多一次进程启动（约 2–5 ms），而且 Go 进程失败时会让 shell 拿不到变量。
- shim 还负责：把 zstyle 转成环境变量（见 0005），注册补全（调用 `cc-login __complete`）。
- 命令本身（list、use、add 等）全部由 Go 二进制处理，shim 不含业务逻辑。

## 后果

- 目录布局的解析在 shim 和 Go 里各写一份，两边要一起改；shim 很短，由测试比对两边对同一个数据目录导出的值。
- 非 zsh 的 shell 需要各自的 shim。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
