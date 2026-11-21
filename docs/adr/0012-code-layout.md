# 0012 代码布局：按命令分文件、统一命名、creds 管凭据目录

状态：采纳

## 背景

2026-11-21 用户觉得代码结构不直观。逐个文件读下来，原因有六处：

1. **文件和命令对不上。** `internal/app` 只有三个文件：`list.go`；`manage.go` 放 use、rename、remove、run，外加 `add.go` 也用的 `authStatus`、`storedEnv`、`lookup`；`add.go` 放 add、renew、adopt、`StrayCredentials` 和 `Prompter`。`ui` 也用的 `Status` 枚举在 `list.go`；错误散在三处。`cmd/cc-login` 里 `cli` 结构体和 list 的输出在 `main.go`，cobra 命令树在 `cli.go`，其余命令的输出在 `commands.go`。跟一个命令（如 remove）要跳 `cli.go` → `commands.go` → `app/manage.go` → `claude/claude.go` 四个文件。
2. **一个词几个意思，一个东西几个名字。** `SignIn` 是 login 类型（`state.SignIn`）、函数（`claude.SignIn`）、注入字段（`App.SignIn`）、方法（`app.signIn`）。`claude.Status`（auth status 的结果）和 `app.Status`（状态枚举）同名；`ui.Login`/`ui.Window` 和 `state.Login`/`anthropic.Window` 同名。命令 `run` 往下叫 `c.launch` → `App.Launch`。`Renew` 返回 `Added`。
3. **凭据目录没有一个包负责。** 路径在 `state.Store.Dir`，key/token 文件名在 `state`，读写在 `creds`，变成环境变量的 `Env`/`StoredEnv` 和要清掉的变量表在 `claude`。`creds` 的包注释只说读 access token。`Env` 和 `StoredEnv` 的区别关系到凭据隔离，却分在两个包里看。
4. **调 claude 要一路传 `path`。** `App` 有 `FindClaude`、`AuthStatus`、`SignIn`、`Logout` 四个字段，每次调用先 `FindClaude()` 再把 path 传下去；有 `authStatus` 包了这一步，`List`、`signIn`、`logout`、`Launch` 仍各自找。
5. **I/O 边界有漏。** `claude.SignIn` 直接读 `os.Stdin`、写 `os.Stdout`；`cmd` 里 `prompter` 和 `confirm` 各自判断终端。
6. **文档和代码对不上。** CLAUDE.md 指向的 ADR 0009 写的 `internal/quota`、lipgloss table、`--format`、`ErrNotSetUp`、TUI 在代码里都没有（0011 去掉了一部分，其余没实现过），0009 没标哪些失效；`ui` 的包注释还写着 TSV。

## 决策

只挪代码和改名，行为不变：testscript、golden 文件和单元测试的断言不改（测试里跟着改名、换注入方式）。

### 包的分工

| 包 | 负责 |
|---|---|
| `internal/state` | 数据根目录：`state.json` 的读写和锁、凭据目录的路径（`Store.Dir`、`NewDir`）、`WriteAtomic`、login 名字的规则 |
| `internal/creds` | 凭据目录里面的东西：key/token 文件（名字、读、写）、sign-in 的 access token（文件或 Keychain）、把一个 login 的凭据变成 claude 的环境（`Env`、`StoredEnv`、要清掉的变量表） |
| `internal/claude` | 找 claude、运行 `claude auth status|login|logout`；不再碰凭据文件 |
| `internal/anthropic` | Anthropic API：检查 key 和 token、读配额 |
| `internal/app` | 每个命令一个函数，不写 stdout、不依赖 cobra |
| `internal/ui` | list 的表格和 JSON |
| `cmd/cc-login` | cobra 命令和输出 |

依赖方向：`app` → `claude`、`creds`、`anthropic`、`state`；`creds` → `state`；`claude` 和 `anthropic` 不依赖其他内部包；`ui` → `app`、`anthropic`、`state`。

### 按命令分文件

- `internal/app`：`app.go`（`App`、`New`、`Prompter`、共用的小函数）、`errors.go`、`status.go`（`Status` 枚举）、`list.go`、`use.go`、`rename.go`、`remove.go`、`run.go`、`add.go`、`renew.go`、`adopt.go`；测试同样按命令分，共用的假实现放 `app_test.go`。
- `cmd/cc-login`：`main.go`（`main`、`execute`、`cli` 结构体、`say`/`errf`）、`root.go`（根命令、`--verbose`、未知命令的提示）、`args.go`（NAME 和 `--` 的检查、补全）、`prompt.go`（终端交互：输入 key/token、确认），每个命令一个文件，命令定义（`func (c *cli) xxxCmd() *cobra.Command`）和它的输出写在一起。

### 统一命名

| 原名 | 新名 | 原因 |
|---|---|---|
| `claude.Status` | `claude.AuthInfo` | 和 `app.Status` 区分 |
| `claude.SignIn` / `claude.Logout` | `claude.AuthLogin` / `claude.AuthLogout` | 和 `claude auth login|logout` 对应，和 login 类型 `state.SignIn` 区分 |
| `app.signIn` | `app.saveSignIn` | 和 `saveKey`、`saveToken` 并列 |
| `claude.Env` / `claude.StoredEnv` | `creds.Env` / `creds.StoredEnv` | 见上表 |
| `state.APIKeyFile` / `state.TokenFile` | `creds.APIKeyFile` / `creds.TokenFile` | 见上表 |
| `App.Launch`、`app.Launch`、`c.launch` | `App.Run`、`app.Exec`、`c.run` | 和命令 `run` 一致；`Exec` 是要 exec 的程序和环境 |
| `main.run` | `main.execute` | 让出 `run` |
| `app.Added` | `app.Saved` | add、adopt、renew 都返回它 |
| `ui.Login` / `ui.Window` | `ui.loginJSON` / `ui.windowJSON` | 只在 JSON 输出里用，和 `claude.statusJSON` 写法一致 |

### claude 用接口注入

`app` 里定义 `Claude` 接口（`Path`、`AuthStatus`、`AuthLogin`、`AuthLogout`），`claude.CLI` 实现它：每次调用时找 claude（`Find` 只是查 PATH）。`App` 的 `FindClaude`、`AuthStatus`、`SignIn`、`Logout` 四个字段换成一个 `Claude` 字段，调用处不再传 path。API 的 `CheckKey`、`CheckToken`、`Usage` 和 `AccessToken` 没有要一路传的参数，仍是函数字段。

### 文档

- 0009 改为「已取代（见 0012）」。它里面仍有效的部分（testscript、golden、errgroup 并发、`signal.NotifyContext`、slog 调试日志、设置只来自环境变量、版本号）在本 ADR 的「沿用」一节列出。
- CLAUDE.md 的分层链接改指 0012；README 的包表按上面更新；`ui` 包注释去掉 TSV。

### 沿用 0009 的部分

- 业务逻辑在 `internal/app`，参数进、结果或带类型的错误出；外部调用（claude、HTTP、Keychain、时钟）通过 `App` 的字段注入。
- login 类型用 `state.Kind`，状态用 `app.Status`，时间用 `time.Time`；表格和 JSON 都从 `app.View` 生成。
- `errgroup.SetLimit` 限制并发，每个 login 单独超时；`signal.NotifyContext` 让 Ctrl-C 取消子进程和请求。
- 命令行端到端测试用 testscript（`cmd/cc-login/testdata/script/*.txtar`），表格用 `internal/ui/testdata/*.golden`，`go test ./internal/ui -update` 重新生成。
- 设置只来自环境变量（README 的设置表）。
- 调试日志用 `log/slog` 写 stderr，`--verbose` 或 `CC_LOGIN_DEBUG=1` 打开，不记 key、token 和它们所在的环境变量。
- `cc-login version` 从 `debug.ReadBuildInfo` 取版本。

## 后果

- 找一个命令的代码：`cmd/cc-login/<命令>.go` 和 `internal/app/<命令>.go` 两个文件。
- 判断一个 claude 进程能看到哪些凭据，只读 `internal/creds/env.go`。
- 文件变多（app 从 4 个到 12 个，cmd 从 5 个到 13 个），每个更短。
- 测试的假 claude 从四个函数变成一个结构体，改动集中在测试的共用部分。
- 第 5 处（`claude.AuthLogin` 直接用 `os.Stdin`/`os.Stdout`）这次不改：它要把终端的行转发给 claude，换成注入 reader/writer 要改转发逻辑，而眼下只有命令行一个调用方。以后加 TUI 时再改。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
