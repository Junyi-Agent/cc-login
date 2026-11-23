---
tags: [cc-login, development]
---

# Developing cc-login

Workflow rules (worktrees, the pre-push review gate, ADRs) are in
[CLAUDE.md](../CLAUDE.md); decisions in [docs/adr](adr/README.md). How the
tool is used is in [[usage]].

## Setup

```sh
brew install go pre-commit      # or: pipx install pre-commit
pre-commit install              # pre-commit and pre-push hooks
go build -o bin/cc-login ./cmd/cc-login
pre-commit run --all-files      # what CI runs, plus govulncheck
go test ./internal/ui -update   # rewrite the golden tables after a deliberate change
```

`cmd/cc-login/testdata/script/*.txtar` run the binary end to end
([testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript))
with a fake `claude`.

To land a feature branch, run `scripts/ship.sh merge branch` in its worktree: it pushes
(the review gate runs), opens the PR, waits for CI, squash merges and removes
the worktree ([ADR 0014](adr/0014-ship-script.md)).

## Releases

Pushing a `v*` tag runs `.github/workflows/release.yml`: goreleaser builds
darwin and linux binaries (amd64, arm64) and publishes them as
`cc-login_<os>_<arch>.tar.gz` with `checksums.txt`, which `install.sh`
downloads ([ADR 0016](adr/0016-release-binaries.md)).

```sh
git tag v0.2.0 origin/main && git push origin v0.2.0
```

## Layout

| Package | What it does |
|---|---|
| `internal/state` | the data root: `state.json` (load, update under the lock), where each credentials directory is |
| `internal/creds` | what a credentials directory holds: a key or token file, a sign-in's access token (`.credentials.json` or the Keychain), and the environment of a claude run with one login |
| `internal/claude` | find claude; run `claude auth status`, `login` and `logout` |
| `internal/anthropic` | the Anthropic API: the plan quota, checking a key |
| `internal/oauth` | a token login's sign-in: the link, and the trade of the pasted code for a token |
| `internal/seal` | the encryption of an export file: a passphrase, PBKDF2-SHA256 and AES-256-GCM ([ADR 0021](adr/0021-export-import.md)) |
| `internal/app` | what each command does, one file per command; no output, no cobra |
| `internal/ui` | the table and the JSON of `list` |
| `cmd/cc-login` | the command line (cobra): one file per command, with its help and what it prints |

The code of a command is in `cmd/cc-login/<command>.go` and
`internal/app/<command>.go` ([ADR 0012](adr/0012-code-layout.md)).

## Plan

1. ~~`list`, `list --json`~~
2. ~~cobra: help, argument checks and completion~~ ([ADR 0006](adr/0006-cobra-and-bubbletea.md))
3. ~~the new structure: `state.json`, `internal/app`,
   typed status, lipgloss table, testscript~~ ([ADR 0008](adr/0008-state-json-layout.md),
   [ADR 0009](adr/0009-code-structure.md))
4. ~~`use`, `rename`, `remove`, `run`, and the `claude` link~~
   ([ADR 0007](adr/0007-launch-claude-by-exec.md))
5. ~~`add`, `renew`, `adopt`~~
6. ~~`version`, debug log~~. No status line or quota refresh: cc-login does
   not do them ([ADR 0010](adr/0010-statusline-and-quota-refresh.md))
7. ~~first run: `add` runs setup-token, errors name the next step, `version`
   checks the link~~ ([ADR 0013](adr/0013-first-run.md))
8. ~~simplify more: no token organization, leaner renew, one expiry column,
   confirmations on stderr~~ ([ADR 0015](adr/0015-simplify-more.md))
9. ~~a token login signs in with OAuth itself, with `user:profile`~~
   ([ADR 0017](adr/0017-own-oauth.md))
10. `cc-login tui` (ADR 0006)
