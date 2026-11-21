# 0008 数据布局：一个 state.json 加按 ID 存放的凭据目录

状态：采纳

## 背景

zsh 版的数据目录：`NAME -> .dirs/ID` 符号链接、`NAME.json` 记录、`current` 存默认 login 的名字、`.pending` 存“没有默认 login 时登录的目录”。改名要同时改链接、记录和 `current`；两个终端同时操作会写出半截状态；悬空链接要专门处理。用户 2026-11-21 决定换成新布局，不再与 zsh 版共用数据目录；凭据继续放在文件里，不进系统钥匙串。

## 决策

- 根目录不变：`$CC_LOGIN_ROOT`，否则 `$XDG_DATA_HOME/claude-logins`（默认 `~/.local/share/claude-logins`）。凭据目录不变：`ROOT/.dirs/ID/`。macOS 上 Claude Code 用这个路径的 SHA-256 前 8 位命名 Keychain 条目，路径一变 login 就未登录，所以迁移不移动、不改名任何凭据目录。
- 元数据集中在 `ROOT/state.json`（0600）：

  ```json
  {
    "version": 1,
    "default": "k3x9",
    "logins": {
      "k3x9": {
        "name": "work",
        "kind": "sign-in",
        "account": "me@example.com",
        "organization": "Example",
        "plan": "team",
        "created_at": "2026-11-21T08:00:00Z",
        "token_expires_at": null
      }
    }
  }
  ```

  键是凭据目录的 ID；`name` 唯一，只允许 `[A-Za-z0-9][A-Za-z0-9._-]*`。`kind` 是 `sign-in`、`token`、`api-key`。
- 凭据仍是文件：`.dirs/ID/api-key`、`.dirs/ID/oauth-token`（0600，目录 0700）；sign-in 的凭据由 Claude Code 自己存（macOS 的 Keychain，其他系统的 `.credentials.json`）。
- 写 state.json：对 `ROOT/.lock` 加 `flock` 排他锁，读出、修改、写到同目录的临时文件、`fsync`、`rename` 覆盖。读不加锁（rename 是原子的）。
- `cc-login migrate`：从旧布局（链接、`NAME.json`、`current`、`.pending`）生成 state.json；旧的链接和记录移到 `ROOT/.legacy-<日期>/`，不删除；state.json 已存在时什么都不做。没有 state.json 而有旧布局时，其他命令报错并提示先运行 `migrate`。
- 改名只改 `name` 字段；删除 login 时先 `claude auth logout`，再删凭据目录和 state.json 里的条目。

## 后果

- 迁移之后 zsh 版读不到新布局，不能再用；迁移前要在 Go 版里能做 zsh 版的所有事。
- 所有元数据的修改都在一个锁、一次原子写里完成，不会出现半截状态。
- `~/.claude/.credentials.json` 和 `.pending` 的收编改成显式命令（`cc-login adopt NAME`），不在每个命令开头检查和提问。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
