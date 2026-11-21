// cc-login: several Claude Code logins on one machine, and the default login
// that new claude launches use. A port of cc-login.zsh; see README.md.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/Junyi-Agent/cc-login/internal/listing"
	"github.com/Junyi-Agent/cc-login/internal/store"
)

var cmds = []string{"list", "use", "add", "run", "renew", "rename", "remove", "help"}

// ported are the commands this binary implements so far; the others still
// run in cc-login.zsh.
var ported = map[string]bool{"list": true, "help": true}

func errf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "cc-login: "+format+"\n", a...)
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd := "list"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch {
	case cmd == "help" || cmd == "-h" || cmd == "--help":
		fmt.Println("usage: cc-login COMMAND [ARGS]")
		fmt.Println()
		fmt.Println("cc-login list [--json]            list the logins; ▸ marks the default, the one new claude launches use.")
		fmt.Println("                                  cc-login without a command does the same. --json prints a JSON array")
		fmt.Println()
		fmt.Printf("only %s are ported to Go so far; the other commands run in cc-login.zsh\n", "list and help")
		return 0
	case strings.HasPrefix(cmd, "-"):
		errf("unknown option %s; see cc-login help", cmd)
		return 1
	case !contains(cmds, cmd):
		errf("unknown command %s; the commands: %s", cmd, strings.Join(cmds, " "))
		return 1
	case !ported[cmd]:
		errf("%s is not ported to Go yet; run it with the zsh cc-login", cmd)
		return 1
	}
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
			continue
		}
		errf("list takes only --json, not %s", a)
		return 1
	}

	s := store.New()
	if !checkSetUp(s) {
		return 1
	}
	return list(s, asJSON)
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// checkSetUp is the read-only part of _cc_login_check: setting cc-login up
// and adopting stray credentials ask questions and move files, which the zsh
// version still does.
func checkSetUp(s *store.Store) bool {
	if !s.SetUp() {
		errf("cc-login is not set up on this machine; set it up with the zsh cc-login")
		return false
	}
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".claude")
	}
	// CLAUDE_CONFIG_DIR is the user's own setting; cc-login reads it as Claude Code does.
	if fi, err := os.Stat(filepath.Join(cfg, ".credentials.json")); err == nil && fi.Mode().IsRegular() { //nolint:gosec // G703: see above
		errf("%s holds a login that a claude run without cc-login wrote; move it into a login with the zsh cc-login", filepath.Join(cfg, ".credentials.json"))
	}
	if id, err := store.ReadTrimmed(filepath.Join(s.Root, ".pending")); err == nil && id != "" && !strings.Contains(id, "/") {
		if _, err := os.Stat(filepath.Join(s.Root, ".dirs", id, ".credentials.json")); err == nil {
			errf("a claude signed in while no login was the default; name that login with the zsh cc-login")
		}
	}
	return true
}

func list(s *store.Store, asJSON bool) int {
	names := s.Names()
	p := listing.NewProber(s)
	logins := p.All(context.Background(), names)
	current, _ := s.Default()

	if asJSON {
		if err := listing.RenderJSON(os.Stdout, logins, current, p.Now); err != nil {
			errf("could not write the JSON: %v", err)
			return 1
		}
		return 0
	}

	st := listing.Style{}
	fd := int(os.Stdout.Fd())
	if term.IsTerminal(fd) && os.Getenv("NO_COLOR") == "" {
		st.Color = true
		st.Theme = listing.Theme()
		if w, _, err := term.GetSize(fd); err == nil {
			st.Cols = w
		}
	}
	if err := listing.Render(os.Stdout, logins, current, p.Now, st); err != nil {
		errf("could not write the list: %v", err)
		return 1
	}
	return 0
}
