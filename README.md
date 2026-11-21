---
tags: [cc-login, golang, claude-code]
---

# cc-login

Keep several Claude Code logins on one machine and switch between them; each
claude gets only its own login's credentials.

## Getting Started

Install:

```sh
go install github.com/Junyi-Agent/cc-login/cmd/cc-login@latest
```

Add a login (cc-login runs `claude setup-token`, then asks you to paste the token):

```sh
cc-login add work
```

Make a plain `claude` go through cc-login (add the `export` line to `~/.zshrc`):

```sh
mkdir -p ~/.local/lib/cc-login
ln -sf "$(command -v cc-login)" ~/.local/lib/cc-login/claude
export PATH="$HOME/.local/lib/cc-login:$PATH"
```

Then, every day:

```sh
claude                 # Claude Code with the default login
cc-login               # list the logins, their quota and expiry
cc-login use personal  # switch the default
```

Everything else: `cc-login --help`. More in [docs/usage.md](docs/usage.md);
contributing in [docs/development.md](docs/development.md).
