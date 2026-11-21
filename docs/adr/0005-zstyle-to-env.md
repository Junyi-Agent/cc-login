# 0005 zstyle 设置改由环境变量传给 Go

状态：已取代（见 0009）

## 背景

zsh 版用 `zstyle ':cc-login:' theme` 和 `zstyle ':cc-login:' quota-cache` 做设置，`theme` 还能用 `zstyle -e` 每次执行一个命令取值。Go 程序读不到 zstyle。

## 决策

- Go 读环境变量：`CC_LOGIN_THEME`（`light` / `dark`）、`CC_LOGIN_QUOTA_CACHE`（缓存文件路径，`~/` 开头时展开）。没有时与 zsh 版相同：主题看 `COLORFGBG`，缓存用 `~/.claude/quota-last.json`。
- zsh shim（0004）在调用 Go 之前，从 zstyle 读出这两个值放进环境（`zstyle -e` 的命令在这时执行），用户的 `~/.zshrc` 不用改。
- `CC_LOGIN_ROOT` 覆盖数据目录，与 `quota-refresh.py` 一致。

## 后果

- 不经过 shim 直接运行 Go 二进制时，zstyle 设置不生效，要自己设环境变量。
- Go 侧已按这两个变量实现（`internal/listing/render.go` 的 `Theme`、`internal/quota/quota.go` 的 `CachePath`）；shim 部分还没写。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
