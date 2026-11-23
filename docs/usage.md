---
tags: [cc-login, usage]
---

# Using cc-login

The short version is in [[README]]. `cc-login --help` and
`cc-login COMMAND --help` describe every command and option.

## Commands

| Command | What it does |
|---|---|
| `list` | the logins, their email, organization, plan, quota, status and expiry; `--json` for scripts. `cc-login` without a command shows the help |
| `run [NAME] [-- ARGS]` | becomes claude (exec) with one login's credentials; without NAME the login of the surrounding claude, else the default |
| `use NAME` | makes a signed-in login the default |
| `rename OLD NEW` | renames; the login stays signed in |
| `remove NAME` | `claude auth logout`, then deletes the login; asks first (`--yes` without a terminal) |
| `version`, `--version` | the version and commit; `version` also says which claude cc-login runs and whether the `claude` link is in place; `--verbose` (or `CC_LOGIN_DEBUG=1`) logs what any command does on stderr, never keys or tokens |
| `completion` | zsh, bash, fish and powershell completion: commands, options, login names |
| `add NAME` | a new login: an OAuth token (the default, `--oauth-token`; see below), an API key (`--api-key`, checked with the API), a token you already have (`--import-token`, [ADR 0020](adr/0020-import-token.md); without `user:profile` it has no account, plan or quota, and its expiry is unknown), or a sign-in (`--sign-in [-- OPTS]`, through `claude auth login`); the key, token or code is shown as `*` while typed; without a terminal it is a line of stdin |
| `renew NAME [-- OPTS]` | signs a login in again so it expires later; a new OAuth token for a token login |
| `doctor` | checks the setup, offline: claude, the `claude` link, keys and tokens set in the shell, `state.json`, each login's files and expiry; prints what to do for each problem; exit 1 when something does not work ([ADR 0019](adr/0019-doctor.md)) |
| `export FILE [NAME...]` | writes token and API key logins (all, or those named) into a new file, encrypted with a passphrase typed twice; sign-in logins stay on this machine ([ADR 0021](adr/0021-export-import.md)) |
| `import FILE` | adds the logins of an export file after asking its passphrase; a name this machine has is skipped; nothing is checked online |
| `adopt NAME` | moves `~/.claude/.credentials.json`, written by a claude run without cc-login, into a new login (`list` says when there is one) |

A token login signs in with OAuth ([ADR 0017](adr/0017-own-oauth.md)):
cc-login prints a link and opens it in the browser; choose the account, allow
access, and paste the code the page shows (`CODE#STATE`). The token lasts a
year and may read the account (`user:inference user:profile`), so `list`
shows its email and claude's `/status` shows the account and organization.
cc-login keeps no refresh token. A token login saved by an older cc-login
(from `claude setup-token`) has no account for claude; `cc-login renew NAME`
replaces its token.

A token login with `user:profile` also has its organization and plan: `add` and
`renew` read them from `/api/oauth/profile` ([ADR 0018](adr/0018-list-org-and-token-quota.md)),
and `list` reads its quota with the token. If the profile cannot be read the
login is saved without them; `renew` tries again.

`list` runs `claude auth status` and the quota request for up to 8 logins at
once; Ctrl-C stops it. The table shows how long until a login expires; `list
--json` has the time.

stdout carries only results: the `list` table or JSON, and `version`. What a
command changed, what to do next, and errors go to stderr. A failed command
exits 1.

## Moving logins to another machine

```sh
cc-login export logins.cc          # on this machine; asks a passphrase twice
cc-login import logins.cc          # on the other one; asks it again
rm logins.cc
```

The file is encrypted (PBKDF2-SHA256 and AES-256-GCM) and written 0600; it
never overwrites a file. Without a terminal the passphrase is a line of
stdin, twice for `export`. The key or token then works on both machines:
`remove` on one does not revoke it on the other. A sign-in login's
credentials hold a refresh token that one machine's refresh would take from
the other, so they stay: on the other machine run `cc-login add NAME
--sign-in`. When the other machine has no default login, the file's default
becomes it.

## Install

`install.sh` (in [[README]]) downloads the latest release for this machine
from [GitHub releases](https://github.com/Junyi-Agent/cc-login/releases),
checks it against the release's `checksums.txt`, and installs it in
`~/.local/bin`; `sh -s -- DIR` installs it in DIR. Run it again to update.
With Go, `go install github.com/Junyi-Agent/cc-login/cmd/cc-login@latest`
builds it instead.

Completion, in `~/.zshrc` after `compinit`:

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
cc-login v1.2.3 (go1.26.11)
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
| `CC_LOGIN_CLAUDE` | the claude to run, instead of the first `claude` in `PATH` |
| `CC_LOGIN_API_URL` | the Anthropic API (tests point it at a local server) |
| `CC_LOGIN_OAUTH_URL` | the server a token login's code is traded at, `https://platform.claude.com` (tests point it at a local server) |
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
    "org": "Example Org",
    "plan": "team",
    "expires_at": "2026-12-21T12:00:00Z",
    "five_hour": {"left_percent": 70, "resets_at": "2026-11-21T15:00:00Z"},
    "seven_day": {"left_percent": 15, "resets_at": "2026-11-23T14:00:00Z"}
  }
]
```

`kind` is `sign-in`, `token` or `api-key`; `status` is `signed-in`,
`expiring` (within a week), `expired` or `signed-out`. The quota is the live
one, read for sign-in logins and for token logins with `user:profile`: `null`
when there is none (an API key login, an older token login, an expired access
token, a failed request); other unknown values
are `null` too.

## Data layout

```
$CC_LOGIN_ROOT, or $XDG_DATA_HOME/claude-logins, or ~/.local/share/claude-logins
├── state.json       the logins and the default (0600; written atomically under .lock)
├── .lock
└── .dirs/ID/        one login's credentials (0700): CLAUDE_SECURESTORAGE_CONFIG_DIR
    ├── api-key      an API key login's key (0600)
    └── oauth-token  a token login's OAuth access token (0600)
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
