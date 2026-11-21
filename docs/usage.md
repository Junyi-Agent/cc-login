---
tags: [cc-login, usage]
---

# Using cc-login

The short version is in [[README]]. `cc-login --help` and
`cc-login COMMAND --help` describe every command and option.

## Commands

| Command | What it does |
|---|---|
| `list` | the logins, their status, quota and expiry; `--json` for scripts. `cc-login` without a command does the same |
| `run [NAME] [-- ARGS]` | becomes claude (exec) with one login's credentials; without NAME the login of the surrounding claude, else the default |
| `use NAME` | makes a signed-in login the default |
| `rename OLD NEW` | renames; the login stays signed in |
| `remove NAME` | `claude auth logout`, then deletes the login; asks first (`--yes` without a terminal) |
| `version`, `--version` | the version and commit; `version` also says which claude cc-login runs and whether the `claude` link is in place; `--verbose` (or `CC_LOGIN_DEBUG=1`) logs what any command does on stderr, never keys or tokens |
| `completion` | zsh, bash, fish and powershell completion: commands, options, login names |
| `add NAME` | a new login: a token from `claude setup-token`, which cc-login runs on the terminal (the default, `--oauth-token`), an API key (`--api-key`), or a sign-in (`--sign-in [-- OPTS]`, through `claude auth login`); the key or token is shown as `*` while typed and checked with the API; without a terminal it is a line of stdin |
| `renew NAME [-- OPTS]` | signs a login in again so it expires later; a new token for a token login |
| `adopt NAME` | moves `~/.claude/.credentials.json`, written by a claude run without cc-login, into a new login (`list` says when there is one) |

`list` runs `claude auth status` and the quota request for up to 8 logins at
once; Ctrl-C stops it.

## Install

`go install` as in [[README]]; releases (`v*` tags) also publish darwin and
linux binaries. Completion, in `~/.zshrc` after `compinit`:

```sh
source <(cc-login completion zsh)
```

## The claude link

cc-login starts claude itself, so a login's key or token is only in that
claude's environment, never in your shell
([ADR 0007](adr/0007-launch-claude-by-exec.md)). The link named `claude` in
[[README]] makes any program that runs `claude` get the default login.

Claude Code's own installer uses `~/.local/bin/claude`; keep the link
elsewhere, in a directory earlier in `PATH`. cc-login finds the real claude
further down `PATH`, skipping itself; `CC_LOGIN_CLAUDE` names it instead.
Check the link in a new shell:

```console
$ cc-login version
cc-login v1.2.3 (go1.26.5)
claude: /Users/me/.local/bin/claude
claude link: /Users/me/.local/lib/cc-login/claude; a plain claude runs with the default login
```

`cc-login run [NAME] -- ARGS` starts claude with a login without the link.

A claude started by cc-login marks its environment with `CC_LOGIN_ID`, so a
claude it starts in turn (`claude -p`, hooks, scripts) stays on the same login
even after `cc-login use` picks another default.

## Settings

| Variable | Meaning |
|---|---|
| `CC_LOGIN_ROOT` | the data directory |
| `CC_LOGIN_THEME` | `light` or `dark`: overrides the terminal background lipgloss detects |
| `CC_LOGIN_CLAUDE` | the claude to run, instead of the first `claude` in `PATH` |
| `CC_LOGIN_API_URL` | the Anthropic API (tests point it at a local server) |
| `CC_LOGIN_DEBUG` | `1`: the debug log, as `--verbose` |
| `NO_COLOR` | no colors |

## `list --json`

```json
[
  {
    "name": "work",
    "default": true,
    "kind": "sign-in",
    "status": "signed-in",
    "account": "me@example.com",
    "plan": "team",
    "expires_at": "2026-12-21T12:00:00Z",
    "five_hour": {"left_percent": 70, "resets_at": "2026-11-21T15:00:00Z"},
    "seven_day": {"left_percent": 15, "resets_at": "2026-11-23T14:00:00Z"}
  }
]
```

`kind` is `sign-in`, `token` or `api-key`; `status` is `signed-in`,
`expiring` (within a week), `expired` or `signed-out`. The quota is the live
one, read for sign-in logins only: `null` when there is none (a token or API
key login, an expired access token, a failed request); other unknown values
are `null` too.

## Data layout

```
$CC_LOGIN_ROOT, or $XDG_DATA_HOME/claude-logins, or ~/.local/share/claude-logins
├── state.json       the logins and the default (0600; written atomically under .lock)
├── .lock
└── .dirs/ID/        one login's credentials (0700): CLAUDE_SECURESTORAGE_CONFIG_DIR
    ├── api-key      an API key login's key (0600)
    └── oauth-token  a token login's token from claude setup-token (0600)
```

A sign-in's credentials are kept by Claude Code: in the macOS Keychain, under a
name made from the directory's path, or in `.credentials.json` in the
directory. See [ADR 0008](adr/0008-state-json-layout.md).

## Moving from the zsh version

cc-login began as a port of `cc-login.zsh` and no longer converts its data
layout. To move those logins over, run `cc-login migrate` from the last
version that has it, once ([ADR 0011](adr/0011-simplify.md)):

```sh
go run github.com/Junyi-Agent/cc-login/cmd/cc-login@6a3cea9 migrate
```
