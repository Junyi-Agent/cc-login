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
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Junyi-Agent/cc-login/internal/listing"
	"github.com/Junyi-Agent/cc-login/internal/store"
)

// app is one run of cc-login: where it writes and which data directory it
// reads.
type app struct {
	out, err io.Writer
	store    *store.Store
	root     *cobra.Command
	ran      bool // the command line passed the checks and the command ran
}

func (a *app) errf(format string, args ...any) {
	_, _ = fmt.Fprintf(a.err, "cc-login: "+format+"\n", args...)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	a := &app{out: stdout, err: stderr, store: store.New()}
	a.root = a.command()

	// cobra reads os.Args when the args are nil.
	if args == nil {
		args = []string{}
	}
	a.root.SetArgs(args)

	c, err := a.root.ExecuteC()
	if err == nil {
		return 0
	}

	// A wrong command line also points to the help of the command.
	a.errf("%v", err)
	if !a.ran {
		_, _ = fmt.Fprintf(a.err, "Run '%s --help' for usage.\n", c.CommandPath())
	}
	return 1
}

// checkSetUp is the read-only part of _cc_login_check: setting cc-login up
// and adopting stray credentials ask questions and move files, which the zsh
// version still does.
func (a *app) checkSetUp() error {
	s := a.store
	if !s.SetUp() {
		return errors.New("cc-login is not set up on this machine; set it up with the zsh cc-login")
	}

	// CLAUDE_CONFIG_DIR is the user's own setting; cc-login reads it as Claude Code does.
	cfg := os.Getenv("CLAUDE_CONFIG_DIR")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".claude")
	}
	stray := filepath.Join(cfg, ".credentials.json")
	if fi, err := os.Stat(stray); err == nil && fi.Mode().IsRegular() { //nolint:gosec // G703: see above
		a.errf("%s holds a login that a claude run without cc-login wrote; move it into a login with the zsh cc-login", stray)
	}

	id, err := store.ReadTrimmed(filepath.Join(s.Root, ".pending"))
	if err == nil && id != "" && !strings.Contains(id, "/") {
		if _, err := os.Stat(filepath.Join(s.Root, ".dirs", id, ".credentials.json")); err == nil {
			a.errf("a claude signed in while no login was the default; name that login with the zsh cc-login")
		}
	}

	return nil
}

// list prints the logins as a table, or as JSON.
func (a *app) list(asJSON bool) error {
	if err := a.checkSetUp(); err != nil {
		return err
	}

	s := a.store
	p := listing.NewProber(s)
	logins := p.All(context.Background(), s.Names())
	current, _ := s.Default()

	if asJSON {
		if err := listing.RenderJSON(a.out, logins, current, p.Now); err != nil {
			return fmt.Errorf("could not write the JSON: %w", err)
		}
		return nil
	}

	// Colors and the width only on a terminal.
	st := listing.Style{}
	if f, ok := a.out.(*os.File); ok && term.IsTerminal(int(f.Fd())) && os.Getenv("NO_COLOR") == "" {
		st.Color = true
		st.Theme = listing.Theme()
		if w, _, err := term.GetSize(int(f.Fd())); err == nil {
			st.Cols = w
		}
	}

	if err := listing.Render(a.out, logins, current, p.Now, st); err != nil {
		return fmt.Errorf("could not write the list: %w", err)
	}
	return nil
}
