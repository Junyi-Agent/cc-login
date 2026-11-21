---
tags: [cc-login, golang, claude-code]
---

# cc-login (Go)

Several Claude Code logins on one machine, and the default login that `claude`
uses. cc-login launches claude itself (`cc-login run`, or a `claude` link to
cc-login), with only that login's credentials in claude's environment.

It began as a port of `cc-login.zsh` and is now its own design: a `state.json`
data layout, claude launched by exec. `cc-login migrate` moves the zsh
version's logins over. Decisions are in
[docs/adr](docs/adr/README.md) (0006–0010).

## Status

| Command | Status |
|---|---|
| `list` | the logins, their status, quota and expiry; `--format json\|tsv\|TEMPLATE` (`--json` = `--format json`) for scripts |
| `run [NAME] [-- ARGS]` | becomes claude (exec) with one login's credentials; without NAME the login of the surrounding claude, else the default |
| `use NAME` | makes a signed-in login the default |
| `rename OLD NEW` | renames; the login stays signed in |
| `remove NAME` | `claude auth logout`, then deletes the login; asks first (`--yes` without a terminal) |
| `migrate` | converts the zsh version's data layout into `state.json` |
| `version`, `--version` | the version and commit; `--verbose` (or `CC_LOGIN_DEBUG=1`) logs what any command does on stderr, never keys or tokens |
| `completion` | zsh, bash, fish and powershell completion: commands, options, login names |
| `add NAME` | a token from `claude setup-token` (the default, `--oauth-token`), an API key (`--api-key`), or a sign-in (`--sign-in [-- OPTS]`, through `claude auth login`); the key or token is shown as `*` while typed and checked with the API; without a terminal it is a line of stdin |
| `renew NAME [-- OPTS]` | signs a login in again so it expires later; a new token for a token login |
| `adopt NAME` | moves `~/.claude/.credentials.json`, written by a claude run without cc-login, into a new login (`list` says when there is one) |

`list` runs `claude auth status` and the quota request for up to 8 logins at
once; Ctrl-C stops it.

## Moving from the zsh version

`cc-login migrate` writes `state.json` from the zsh version's links, records,
`current` and `.pending`. The credentials directories stay where they are, so
every login stays signed in; the old links and records move into
`.legacy-<time>/` in the data directory. After that the zsh `cc-login` cannot
read the logins: stop loading `cc-login.zsh` in your shell, and start claude
through cc-login (next section). Scripts that read the old links and records
(a status line, a quota refresh) stop finding the logins; see
[ADR 0010](docs/adr/0010-statusline-and-quota-refresh.md).

## Starting claude

cc-login starts claude itself, so a login's key or token is only in that
claude's environment, never in your shell
([ADR 0007](docs/adr/0007-launch-claude-by-exec.md)). Either:

- a link named `claude` to cc-login, in a directory early in `PATH`. Any
  program that runs `claude` then gets the default login:

  ```sh
  mkdir -p ~/.local/lib/cc-login
  ln -sf "$(command -v cc-login)" ~/.local/lib/cc-login/claude
  export PATH="$HOME/.local/lib/cc-login:$PATH"   # in ~/.zshrc, before other PATH changes
  ```

  (Claude Code's own installer uses `~/.local/bin/claude`; keep the link
  elsewhere.) cc-login finds the real claude further down `PATH`, skipping
  itself; `CC_LOGIN_CLAUDE` names it instead.
- or an alias: `alias claude='cc-login run --'`.

A claude started by cc-login marks its environment with `CC_LOGIN_ID`, so a
claude it starts in turn (`claude -p`, hooks, scripts) stays on the same login
even after `cc-login use` picks another default.

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
directory. See [ADR 0008](docs/adr/0008-state-json-layout.md).

### `list --format json`

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
    "seven_day": {"left_percent": 15, "resets_at": "2026-11-23T14:00:00Z"},
    "quota_source": "live"
  }
]
```

`kind` is `sign-in`, `token` or `api-key`; `status` is `signed-in`,
`expiring` (within a week), `expired` or `signed-out`; `quota_source` is
`live`, `cache` (then `cached_at` is set) or `null`. Unknown values are `null`.
`--format tsv` prints name, default, kind, status, account, plan, expires_at
and the percent left of the two windows; `--format '{{.Name}}'` applies a Go
template to each login (the fields of the JSON object, in Go names).

## Settings

| Variable | Meaning |
|---|---|
| `CC_LOGIN_ROOT` | the data directory |
| `CC_LOGIN_THEME` | `light` or `dark`: overrides the terminal background lipgloss detects |
| `CC_LOGIN_QUOTA_CACHE` | the quota cache file (default `~/.claude/quota-last.json`) |
| `CC_LOGIN_CLAUDE` | the claude to run, instead of the first `claude` in `PATH` |
| `CC_LOGIN_API_URL` | the Anthropic API (tests point it at a local server) |
| `CC_LOGIN_DEBUG` | `1`: the debug log, as `--verbose` |
| `NO_COLOR` | no colors |

## Install

```sh
go install github.com/Junyi-Agent/cc-login/cmd/cc-login@latest
```

Releases (`v*` tags) also publish darwin and linux binaries. Completion, in
`~/.zshrc` after `compinit`:

```sh
source <(cc-login completion zsh)
```

## Development

```sh
brew install go pre-commit      # or: pipx install pre-commit
pre-commit install              # pre-commit and pre-push hooks
go build -o bin/cc-login ./cmd/cc-login
pre-commit run --all-files      # what CI runs
go test ./internal/ui -update   # rewrite the golden tables after a deliberate change
```

`cmd/cc-login/testdata/script/*.txtar` run the binary end to end
([testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript))
with a fake `claude`. Workflow rules (worktrees, the pre-push review gate,
ADRs) are in [CLAUDE.md](CLAUDE.md); decisions in [docs/adr](docs/adr/README.md).

## Layout

| Package | What it does |
|---|---|
| `internal/state` | `state.json`: load, update under the lock, migrate from the zsh layout |
| `internal/claude` | find claude, the environment of one login, `claude auth status` |
| `internal/creds` | a login's key or token file; a sign-in's access token from `.credentials.json` or the Keychain |
| `internal/anthropic` | the Anthropic API: the plan quota |
| `internal/quota` | quota windows and the quota cache |
| `internal/app` | one function per command; no output, no cobra |
| `internal/ui` | the table (lipgloss), JSON, TSV and template output |
| `cmd/cc-login` | the command line (cobra) |

## Plan

1. ~~`list`, `list --json`~~
2. ~~cobra: help, argument checks and completion~~ ([ADR 0006](docs/adr/0006-cobra-and-bubbletea.md))
3. ~~the new structure: `state.json` and `cc-login migrate`, `internal/app`,
   typed status, lipgloss table, testscript, `list --format`~~ ([ADR 0008](docs/adr/0008-state-json-layout.md),
   [ADR 0009](docs/adr/0009-code-structure.md))
4. ~~`use`, `rename`, `remove`, `run`, and the `claude` link~~
   ([ADR 0007](docs/adr/0007-launch-claude-by-exec.md))
5. ~~`add`, `renew`, `adopt`~~
6. ~~`version`, debug log~~; the status line and the quota refresh wait for a
   decision ([ADR 0010](docs/adr/0010-statusline-and-quota-refresh.md)): the
   local scripts that do them read the zsh layout, so they stop finding logins
   after `cc-login migrate`
7. `cc-login tui` (ADR 0006)
