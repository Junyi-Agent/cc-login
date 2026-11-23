# 0022 cc-login 保留现在的命令结构；ship.sh 和 install.sh 改成 kubectl 式

状态：采纳

## 背景

2026-11-23 的仓库审查（`/jy-repo-review`）按 kubectl 式结构检查命令行入口：`<tool> <动词> <资源> [名字] [flags]`，不同资源用同一套动词，输出格式用 `-o`/`--output` 选。cc-login 的子命令不满足这个结构：

- `list [--json]` 没有资源名，输出格式用 `--json`，没有 `-o`。
- `use NAME`、`add NAME`、`renew NAME`、`rename OLD NEW`、`remove NAME`、`adopt NAME`、`run [NAME] [-- ARGS]`、`export FILE [NAME...]`、`import FILE` 都是动词后面直接跟名字或文件，没有资源名；`add`、`remove` 也不在 kubectl 那套动词（create、delete）里。

`doctor`、`version`、`completion` 和 kubectl 的同名命令形状一致，不在问题之列。

另外两个按名字调用的入口也不满足：`scripts/ship.sh [--keep]` 和 `install.sh [DIR]`（`curl … | sh -s -- DIR`）都没有动词和资源。

## 决策

### cc-login：保留现在的命令结构，豁免 kubectl 式结构的要求

- 豁免对象：`cc-login` 的全部子命令，即 `list`、`use`、`add`、`run`、`renew`、`rename`、`remove`、`adopt`、`export`、`import`，以及 `list` 用 `--json` 选输出格式、不注册 `-o`/`--output`。
- 理由：cc-login 只管理 login 这一种资源，每个命令都是对 login 的操作。加上资源名（`get logins`、`delete login NAME`）只多打一个词，不带来区分；改名会让现有用法、文档和脚本全部失效。
- 批准人：Junyi，2026-11-23。

以后如果出现第二种资源，再重新考虑这个决定。

### ship.sh 和 install.sh：改成 kubectl 式

用户决定（2026-11-23）这两个脚本按 kubectl 式结构改，不豁免：

- `scripts/ship.sh merge branch [--keep]`：动作和资源都写出来；不带参数或动作、资源不对时打印用法、退出码 2，`help`/`-h`/`--help` 打印用法、退出码 0。
- `install.sh install binary [DIR]`：README 的安装命令变成 `curl -fsSL …/install.sh | sh -s -- install binary`；不带参数时打印用法、退出码 2，什么都不装。这改了 [[0016-release-binaries]] 里 install.sh 的调用形式，0016 的其余决定不变。

两个脚本都不输出给程序读的结果，不需要 `-o`。

## 后果

- cc-login 的命令行和文档不变，审查和 `jy-repo-constrain-check` 按这篇 ADR 不再上报它的子命令。
- `curl … | sh` 不带参数不再安装：旧的一行命令会打印用法并失败，要换成 README 里的新命令。ship.sh 的旧调用 `scripts/ship.sh` 同样只打印用法。
- 新增子命令沿用现在的写法（动词直接跟名字），不受 kubectl 式结构约束；要新增第二种资源时，先改这篇 ADR。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
