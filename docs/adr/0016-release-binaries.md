# 0016 用 GitHub release 的二进制安装

状态：已验证

## 背景

README 的安装方法是 `go install`，要求用户装 Go。`release.yml` 在推 `v*` tag 时用 goreleaser 发布 darwin 和 linux 的二进制，但一直没有推过 tag，所以没有 release。2026-11-22 用户要求在 GitHub 自动发布二进制，Getting Started 教用户安装二进制。

## 决策

- 发布方式不变：推 `v*` tag 触发 `release.yml`，goreleaser 构建 darwin/linux × amd64/arm64。
- 包名去掉版本号：`cc-login_<os>_<arch>.tar.gz`（`.goreleaser.yaml` 的 `name_template`），这样 `releases/latest/download/<包名>` 是固定的地址，总是最新版本。
- 仓库根目录加 `install.sh`：按 `uname` 选包，下载包和 `checksums.txt`，SHA-256 不符就不安装；装到 `~/.local/bin`（Claude Code 的安装器也用这个目录，通常已在 PATH 里），参数可以指定别的目录；不在 PATH 里时提示加到 `~/.zshrc`。不改 shell 配置，不装 `claude` 链接。
- README 的 Getting Started 用 `curl -fsSL .../install.sh | sh`；`go install` 留在 `docs/usage.md` 作为另一种方式。
- 合并之后推 `v0.1.0`，产生第一个 release。

## 后果

- 用户不需要 Go。
- 更新就是再跑一次 `install.sh`。
- 每次发布要人或 agent 推一个 tag；合并到 `main` 不会自动发布。
- `install.sh` 没有自动测试：它下载真实的 release，只能在发布后真实运行验证。
- 二进制没有签名和公证：用 `curl` 下载的文件不带 quarantine 属性，macOS 不拦；从浏览器下载的要自己 `xattr -d com.apple.quarantine`。

## 验证

验证人：Junyi
日期：2026-11-22
测了什么：推 v0.2.0 tag 后，用 install.sh 安装 release 二进制；cc-login version 显示 0.2.0；重做 claude 链接后 cc-login doctor 全部通过。
结果：通过。
