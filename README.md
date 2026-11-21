---
tags: [cc-login, golang, claude-code]
---

# cc-login

Keep several Claude Code logins on one machine and switch between them; each
claude gets only its own login's credentials.

## Getting Started

Install (macOS or Linux; the binary goes to `~/.local/bin`):

```sh
curl -fsSL https://raw.githubusercontent.com/Junyi-Agent/cc-login/main/install.sh | sh
```

Add a login (cc-login opens a sign-in link in the browser, then asks you to paste the code it shows):

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
cc-login list          # the logins, their quota and expiry
cc-login use personal  # switch the default
```

Everything else: `cc-login --help`. More in [docs/usage.md](docs/usage.md);
contributing in [docs/development.md](docs/development.md).
