# 0021 export 和 import：用口令加密的文件把 login 搬到另一台机器

状态：采纳

## 背景

换机器或多一台机器时，每个 login 都要重新 `add`：token login 要再走一遍浏览器授权，API key 要再从 Console 找出来。用户 2026-11-22 要求加 export 和 import，并决定：

- 只带 token 和 API key login。sign-in 的凭据在 macOS Keychain 里，含会轮换的 refresh token：两台机器用同一份，先刷新的一台会让另一台掉线。sign-in login 在新机器上用 `add --sign-in` 重新登录。
- 导出文件用口令加密。

## 决策

### 命令

- `cc-login export FILE [NAME...]`：把 token 和 API key login 写进 FILE。不给 NAME 时导出全部 token 和 API key login，跳过 sign-in login 并在 stderr 列出；给的 NAME 是 sign-in login 时报错，什么也不写。一个可导出的 login 都没有时报错。
- `cc-login import FILE`：把 FILE 里的 login 加到这台机器。
- FILE 已存在时 export 报错，不覆盖（`O_EXCL`）；新文件 0600。
- 口令从 stdin 读：终端上显示成 `*`，export 要输两遍；不是终端时每遍读一行 stdin。不放在命令行参数或环境变量里。export 要求口令至少 8 个字符。
- 先检查 login 和读出 key/token，再问口令，口令之后不再有会失败的检查。

### 文件格式

外层是 JSON，只有解密需要的参数：

```json
{
  "cc_login_export": 1,
  "kdf": "pbkdf2-sha256",
  "iterations": 600000,
  "salt": "base64",
  "nonce": "base64",
  "data": "base64"
}
```

- 密钥：标准库 `crypto/pbkdf2`（SHA-256，16 字节随机 salt）从口令导出 32 字节；加密：AES-256-GCM，12 字节随机 nonce。不加新依赖。
- GCM 的附加数据是 `cc-login-export 1 pbkdf2-sha256 <iterations>`，改了外层参数就解不开。
- `iterations` 只接受 100000 到 10000000，挡住让 import 算很久的文件。
- 解开的 `data` 是：

  ```json
  {
    "version": 1,
    "exported_at": "2026-11-22T05:00:00Z",
    "default": "work",
    "logins": [
      {"name": "work", "kind": "token", "account": "me@example.com", "scopes": ["user:inference", "user:profile"], "token_expires_at": "2027-11-22T05:00:00Z", "created_at": "...", "secret": "sk-ant-oat01-..."}
    ]
  }
  ```

  login 的字段和 state.json 一样（[0008](0008-state-json-layout.md)），加上 `secret`；`default` 是默认 login 的名字，不是导出的就为空。不带凭据目录的 ID：import 给每个 login 新建目录。
- 加解密在新包 `internal/seal`（只管口令和字节，不知道 login）；文件内容和命令在 `internal/app/export.go`、`import.go`。

### import

- 口令错或文件被改过：报错「wrong passphrase, or the file was changed」，什么也不改。
- 每个 login 检查名字（`state.CheckName`）、`kind`（只收 token、api-key）和 `secret` 的前缀（`sk-ant-oat`、`sk-ant-`），一个不合格就整个文件报错，什么也不改。
- 名字在这台机器上已有的 login 跳过，在 stderr 说明（先 rename 或 remove，再 import）。不覆盖已有 login。
- 不连网：元数据原样带过来，不重新检查 key 和 token。
- 先建凭据目录、写 key/token 文件，再在一次 `Store.Update` 里加进 state.json；失败时删掉新建的目录。
- 这台机器没有默认 login 时：文件的默认 login 被导入了就用它，否则用导入的第一个（按名字）。
- 日志只记名字和个数，不记口令、key、token。

## 后果

- token 和 API key login 一条命令搬到新机器，不用重新授权。
- 同一个 token 或 key 在两台机器上同时有效；在一台上 `remove` 不会让另一台的失效。token 的过期时间照旧，`renew` 只换这台机器的 token。
- 导出文件的安全取决于口令：拿到文件的人可以离线猜口令。PBKDF2 不抗 GPU，比 scrypt/argon2 弱，换来的是不加依赖；输出提醒用户用完删掉文件。
- sign-in login 搬不过去，要在新机器上重新登录。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
