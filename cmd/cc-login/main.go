// cc-login: several Claude Code logins on one machine, and the default login
// that new claude launches use. A port of cc-login.zsh; see README.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Junyi-Agent/cc-login/internal/listing"
	"github.com/Junyi-Agent/cc-login/internal/store"
)

// ported are the commands this binary implements so far; the others pass the
// argument checks and then say to use cc-login.zsh.
var ported = map[string]bool{"list": true, "help": true, "completion": true}

// app is one run of cc-login: where it writes and which data directory it
// reads.
type app struct {
	out, err io.Writer
	store    *store.Store
	root     *cobra.Command
}

// exitCode ends a run with that code; the message is already printed.
type exitCode int

func (e exitCode) Error() string { return "exit status " + strconv.Itoa(int(e)) }

func (a *app) errf(format string, args ...any) {
	_, _ = fmt.Fprintf(a.err, "cc-login: "+format+"\n", args...)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	a := &app{out: stdout, err: stderr, store: store.New()}
	root := a.command()
	a.root = root
	if args == nil {
		args = []string{} // cobra reads os.Args when the args are nil
	}
	root.SetArgs(args)
	err := root.Execute()
	var code exitCode
	switch {
	case errors.As(err, &code):
		return int(code)
	case err != nil:
		a.errf("%v", err)
		return 1
	}
	return 0
}

// dispatch runs cmd with its arguments; cmd is "" when cc-login has no
// arguments.
func (a *app) dispatch(cmd string, args []string) error {
	if cmd == "" {
		cmd = "list" // cc-login, and cc-login "" as ${1:-list} in zsh
	}
	switch cmd {
	case "help", "-h", "--help":
		switch {
		case len(args) == 0:
			printUsage(a.out, "")
		case len(args) == 1 && usageText[args[0]] != "":
			printUsage(a.out, args[0])
		default:
			a.errf("no command %s; the commands: %s", args[0], strings.Join(cmds, " "))
			return exitCode(1)
		}
		return nil
	case "list", "use", "add", "run", "renew", "rename", "remove", "completion":
	default:
		if strings.HasPrefix(cmd, "-") {
			a.errf("unknown option %s; see cc-login help", cmd)
			return exitCode(1)
		}
		a.errf("unknown command %s; the commands: %s", cmd, strings.Join(cmds, " "))
		if _, ok := a.store.Dir(cmd); ok {
			a.errf("to make login %s the default, run: cc-login use %s", cmd, cmd)
		}
		return exitCode(1)
	}

	inv, code, done := a.parse(cmd, args)
	switch {
	case done && code == 0:
		return nil
	case done:
		return exitCode(code)
	case !ported[cmd]:
		a.errf("%s is not ported to Go yet; run it with the zsh cc-login", cmd)
		return exitCode(1)
	case cmd == "completion":
		return a.root.GenZshCompletion(a.out)
	}
	if !a.checkSetUp() {
		return exitCode(1)
	}
	// list is the only ported command that gets here.
	return a.list(inv.json)
}

// checkSetUp is the read-only part of _cc_login_check: setting cc-login up
// and adopting stray credentials ask questions and move files, which the zsh
// version still does.
func (a *app) checkSetUp() bool {
	s := a.store
	if !s.SetUp() {
		a.errf("cc-login is not set up on this machine; set it up with the zsh cc-login")
		return false
	}
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".claude")
	}
	// CLAUDE_CONFIG_DIR is the user's own setting; cc-login reads it as Claude Code does.
	if fi, err := os.Stat(filepath.Join(cfg, ".credentials.json")); err == nil && fi.Mode().IsRegular() { //nolint:gosec // G703: see above
		a.errf("%s holds a login that a claude run without cc-login wrote; move it into a login with the zsh cc-login", filepath.Join(cfg, ".credentials.json"))
	}
	if id, err := store.ReadTrimmed(filepath.Join(s.Root, ".pending")); err == nil && id != "" && !strings.Contains(id, "/") {
		if _, err := os.Stat(filepath.Join(s.Root, ".dirs", id, ".credentials.json")); err == nil {
			a.errf("a claude signed in while no login was the default; name that login with the zsh cc-login")
		}
	}
	return true
}

func (a *app) list(asJSON bool) error {
	s := a.store
	p := listing.NewProber(s)
	logins := p.All(context.Background(), s.Names())
	current, _ := s.Default()

	if asJSON {
		if err := listing.RenderJSON(a.out, logins, current, p.Now); err != nil {
			a.errf("could not write the JSON: %v", err)
			return exitCode(1)
		}
		return nil
	}

	st := listing.Style{}
	if f, ok := a.out.(*os.File); ok && term.IsTerminal(int(f.Fd())) && os.Getenv("NO_COLOR") == "" {
		st.Color = true
		st.Theme = listing.Theme()
		if w, _, err := term.GetSize(int(f.Fd())); err == nil {
			st.Cols = w
		}
	}
	if err := listing.Render(a.out, logins, current, p.Now, st); err != nil {
		a.errf("could not write the list: %v", err)
		return exitCode(1)
	}
	return nil
}
