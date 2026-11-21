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
	if wantsHelp(args) {
		printUsage(a.out, cmd)
		return inv, 0, true
	}

	bad := func() (invocation, int, bool) {
		printUsage(a.err, cmd)
		return invocation{}, 1, true
	}

	// Every command but list starts with a NAME.
	if cmd != "list" && !isName(args, 0) {
		return bad()
	}

	inv = invocation{cmd: cmd}
	switch cmd {
	case "list":
		if len(args) > 1 || len(args) == 1 && args[0] != "--json" {
			return bad()
		}
		inv.json = len(args) == 1

	case "use", "remove", "completion":
		if len(args) != 1 || cmd == "completion" && args[0] != "zsh" {
			return bad()
		}
		inv.args = args

	case "rename":
		if len(args) != 2 || !isName(args, 1) {
			return bad()
		}
		inv.args = args

	case "add", "run", "renew":
		var ok bool
		if inv, ok = a.parseLogin(cmd, args); !ok {
			return bad()
		}
	}

	return inv, 0, false
}

// parseLogin checks the arguments of add, run and renew: NAME; for add, an
// option that says what to save; then the options for claude, after --.
// It prints the error, if any; the caller prints the usage.
func (a *app) parseLogin(cmd string, args []string) (invocation, bool) {
	inv := invocation{cmd: cmd, name: args[0], kind: kindAuth}
	args = args[1:]

	// add saves a token unless --sign-in or --api-key says otherwise.
	if cmd == "add" {
		inv.kind = kindToken
		if len(args) > 0 {
			switch opt := args[0]; opt {
			case "--api-key", "--oauth-token":
				if opt == "--api-key" {
					inv.kind = kindKey
				}
				if len(args) > 1 {
					a.errf("add NAME %s takes nothing more", opt)
					return inv, false
				}
				return inv, true

			case "--sign-in":
				inv.kind = kindAuth
				args = args[1:]
			}
		}
	}

	if len(args) == 0 {
		return inv, true
	}

	if args[0] != "--" {
		// cc-login.zsh also suggests --sign-in for run and renew, which do
		// not take it; Go suggests it for add --sign-in only.
		signIn := ""
		if cmd == "add" && inv.kind == kindAuth {
			signIn = " --sign-in"
		}
		a.errf("unexpected argument %s; options for claude go after --: cc-login %s %s%s -- %s",
			args[0], cmd, inv.name, signIn, strings.Join(args, " "))
		return inv, false
	}

	if inv.kind == kindToken {
		a.errf("options for claude auth login need --sign-in: cc-login add %s --sign-in %s",
			inv.name, strings.Join(args, " "))
		return inv, false
	}

	inv.args = args[1:]
	return inv, true
}

// wantsHelp reports whether -h or --help comes before any --.
func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

// isName reports whether args[i] is there and can be a NAME: not empty and
// not an option.
func isName(args []string, i int) bool {
	return i < len(args) && args[i] != "" && !strings.HasPrefix(args[i], "-")
}
