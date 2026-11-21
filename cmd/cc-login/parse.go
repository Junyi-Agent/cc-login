package main

import "strings"

// Login kinds for add: a sign-in with claude auth login, a setup-token token,
// or an API key.
const (
	kindAuth  = "auth"
	kindToken = "token"
	kindKey   = "key"
)

// invocation is a command line that passed the checks.
type invocation struct {
	cmd  string
	name string   // add, run, renew: the login
	args []string // use, remove, rename: the names; add, run, renew: the options for claude, after --
	kind string   // add: kindAuth, kindToken or kindKey
	json bool     // list --json
}

// parse checks the arguments of cmd in full, as cc-login() in cc-login.zsh
// does before _cc_login_check, so a wrong command line asks nothing and
// changes nothing. done is true when parse printed the usage or an error;
// code is then the exit code.
func (a *app) parse(cmd string, args []string) (inv invocation, code int, done bool) {
	// -h or --help before any -- prints the usage of the command.
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "-h" || arg == "--help" {
			printUsage(a.out, cmd)
			return inv, 0, true
		}
	}
	bad := func() (invocation, int, bool) {
		printUsage(a.err, cmd)
		return invocation{}, 1, true
	}
	isOpt := func(s string) bool { return strings.HasPrefix(s, "-") }

	// A NAME argument: present and not an option.
	if cmd != "list" && (len(args) == 0 || args[0] == "" || isOpt(args[0])) {
		return bad()
	}
	inv = invocation{cmd: cmd, kind: kindAuth}
	switch cmd {
	case "list":
		if len(args) > 1 || len(args) == 1 && args[0] != "--json" {
			return bad()
		}
		inv.json = len(args) == 1
	case "use", "remove", "rename", "completion":
		want := map[string]int{"use": 1, "remove": 1, "rename": 2, "completion": 1}[cmd]
		if len(args) != want ||
			cmd == "rename" && (args[1] == "" || isOpt(args[1])) ||
			cmd == "completion" && args[0] != "zsh" {
			return bad()
		}
		inv.args = args
	case "add", "run", "renew":
		inv.name, args = args[0], args[1:]
		// add saves a token unless --sign-in or --api-key says otherwise.
		if cmd == "add" {
			inv.kind = kindToken
		}
		if cmd == "add" && len(args) > 0 && (args[0] == "--api-key" || args[0] == "--oauth-token") {
			if args[0] == "--api-key" {
				inv.kind = kindKey
			}
			opt := args[0]
			if len(args) > 1 {
				a.errf("add NAME %s takes nothing more", opt)
				return bad()
			}
			args = nil
		} else {
			if cmd == "add" && len(args) > 0 && args[0] == "--sign-in" {
				inv.kind = kindAuth
				args = args[1:]
			}
			if len(args) > 0 {
				switch {
				case args[0] != "--":
					// cc-login.zsh also suggests --sign-in for run and renew,
					// which do not take it; Go suggests it for add --sign-in only.
					signIn := ""
					if cmd == "add" && inv.kind == kindAuth {
						signIn = " --sign-in"
					}
					a.errf("unexpected argument %s; options for claude go after --: cc-login %s %s%s -- %s",
						args[0], cmd, inv.name, signIn, strings.Join(args, " "))
					return bad()
				case inv.kind == kindToken:
					a.errf("options for claude auth login need --sign-in: cc-login add %s --sign-in %s",
						inv.name, strings.Join(args, " "))
					return bad()
				}
				args = args[1:]
			}
		}
		inv.args = args
	}
	return inv, 0, false
}
