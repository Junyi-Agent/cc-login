package main

import (
	"fmt"
	"io"
)

// cmds are the commands, in the order the usage lists them; the error for an
// unknown command names them. completion is Go only.
var cmds = []string{"list", "use", "add", "run", "renew", "rename", "remove", "completion", "help"}

// usageText is the usage of each command, as _cc_login_help in cc-login.zsh,
// plus the Go-only list --json and completion.
var usageText = map[string]string{
	"list": `cc-login list [--json]            list the logins; ▸ marks the default, the one new claude launches use.
                                  cc-login without a command does the same. --json prints a JSON array`,
	"use": `cc-login use NAME                 make login NAME the default`,
	"add": `cc-login add NAME                 save a token from claude setup-token (sk-ant-oat...) as login NAME, or add
                                  it to login NAME, where it overrides the stored sign-in: read without echo,
                                  checked with the Anthropic API; it lasts a year. --oauth-token does the same.
                                  A new login becomes the default when the machine has none
cc-login add NAME --sign-in [-- OPTS...]
                                  sign login NAME in with claude auth login, which keeps the sign-in in the
                                  macOS Keychain (OPTS: its options, such as --email ADDR)
cc-login add NAME --api-key       save an Anthropic API key (sk-ant-...) as login NAME: read without echo,
                                  checked with the Anthropic API`,
	"run": `cc-login run NAME [-- ARGS...]    one claude launch with login NAME, without changing the default
                                  (ARGS: options and arguments for claude)`,
	"renew": `cc-login renew NAME [-- OPTS...]  sign login NAME in again with claude auth login, also while it is
                                  signed in, so that it expires later (OPTS: as for add). For a token
                                  login, save a new token from claude setup-token`,
	"rename": `cc-login rename OLD NEW           rename login OLD to NEW`,
	"remove": `cc-login remove NAME              sign login NAME out (claude auth logout) and delete it`,
	"help": `cc-login help [COMMAND]           print the usage of all commands, or of COMMAND.
                                  cc-login COMMAND -h does the same`,
	"completion": `cc-login completion zsh           print the zsh completion script; load it with
                                  source <(cc-login completion zsh)`,
}

// printUsage prints the usage of cmd, or of all commands when cmd is "", as
// _cc_login_usage.
func printUsage(w io.Writer, cmd string) {
	if cmd != "" {
		_, _ = fmt.Fprintf(w, "usage:\n%s\n", usageText[cmd])
		return
	}
	_, _ = fmt.Fprint(w, "usage: cc-login COMMAND [ARGS]\n\n")
	for _, c := range cmds {
		_, _ = fmt.Fprintln(w, usageText[c])
	}
}
