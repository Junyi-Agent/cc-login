# 0010 statusline 和 quota-refresh 成为 cc-login 的子命令

状态：采纳

## 背景

Claude Code 的状态栏和定时刷新配额由两个 Python 脚本完成（状态栏脚本和一个 launchd 定时任务）。它们各自解析 login 的目录布局。数据布局改成 state.json（0008）之后，这些脚本也得跟着改；布局最好只在一处实现。

## 决策

- `cc-login quota-refresh`：对每个已登录的 login 请求配额，写入配额缓存（原子写，0600）。由 launchd 或 cron 定时运行；网络错误只影响这一次，不清空旧缓存。
- `cc-login statusline`：从 stdin 读 Claude Code 给状态栏的 JSON，只读 state.json 和配额缓存，不发网络请求、不启动子进程，输出一行状态。读缓存和刷新缓存分开，状态栏不会被网络拖慢。
- 两个 Python 脚本在这两个命令可用之后由用户自己替换；仓库里给出 launchd plist 和 Claude Code `statusLine` 设置的示例。

## 后果

- 数据布局只在 Go 里实现一次。
- 状态栏显示的是上一次刷新时的配额，刷新间隔决定新旧。

## 验证

（只由人类填写。状态改成「已验证」时填：验证人、日期、测了什么、结果。）
