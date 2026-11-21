---
tags: [cc-login, golang, claude-code]
---

# cc-login (Go)

Several Claude Code logins on one machine, and the default login that `claude`
uses. cc-login launches claude itself (`cc-login run`, or a `claude` link to
cc-login), with only that login's credentials in claude's environment.

It began as a port of `cc-login.zsh` and is now its own design: a `state.json`
data layout, claude launched by exec. The redesign is in progress; until it is
done, the zsh version keeps working on the old layout. Decisions are in
[docs/adr](docs/adr/README.md) (0006–0010).

## Status

| Command | Status |
|---|---|
| `list` | the logins, their status, quota and expiry; `--format json\|tsv\|TEMPLATE` (`--json` = `--format json`) for scripts |
| `migrate` | converts the zsh version's data layout into `state.json` |
| `completion` | zsh, bash, fish and powershell completion: commands, options, login names |
| `use` `add` `run` `renew` `rename` `remove` | not yet: they check their arguments, then say to use the zsh version |

`list` runs `claude auth status` and the quota request for up to 8 logins at
once; Ctrl-C stops it.

## Moving from the zsh version

`cc-login migrate` writes `state.json` from the zsh version's links, records,
`current` and `.pending`. The credentials directories stay where they are, so
every login stays signed in; the old links and records move into
`.legacy-<time>/` in the data directory. After that the zsh `cc-login` cannot
read the logins: stop loading `cc-login.zsh` in your shell. Until `use`, `add`
and `run` exist here, keep the zsh version and do not migrate yet.

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
4. `use`, `rename`, `remove`, `run`, and the `claude` link
   ([ADR 0007](docs/adr/0007-launch-claude-by-exec.md))
5. `add`, `renew`, `adopt`
6. `statusline`, `quota-refresh`, `version`, debug log
   ([ADR 0010](docs/adr/0010-statusline-and-quota-refresh.md))
7. `cc-login tui` (ADR 0006)
