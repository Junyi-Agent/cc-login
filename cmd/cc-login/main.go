// cc-login: several Claude Code logins on one machine, and the default login
// that claude uses. See README.md.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Junyi-Agent/cc-login/internal/app"
	"github.com/Junyi-Agent/cc-login/internal/ui"
)

// cli is one run of cc-login: the app, and where the output goes.
type cli struct {
	app      *app.App
	out, err io.Writer
	root     *cobra.Command
	ran      bool // the command line passed the checks and the command ran

	// exec replaces this process with a program; tests replace it.
	exec func(path string, argv, env []string) error
}

func main() {
	// Ctrl-C cancels what runs: claude auth status, the quota requests.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	// Installed as claude (a link named claude to cc-login, ADR 0007), it
	// runs claude with the login: cc-login run -- ARGS.
	args := os.Args[1:]
	if filepath.Base(os.Args[0]) == "claude" {
		args = append([]string{"run", "--"}, args...)
	}

	code := run(ctx, args, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	c := &cli{app: app.New(), out: stdout, err: stderr, exec: syscall.Exec}
	c.root = c.command()

	// cobra reads os.Args when the args are nil.
	if args == nil {
		args = []string{}
	}
	c.root.SetArgs(args)

	cmd, err := c.root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}

	// A wrong command line also points to the help of the command.
	c.errf("%v", err)
	if !c.ran {
		_, _ = fmt.Fprintf(c.err, "Run '%s --help' for usage.\n", cmd.CommandPath())
	}
	return 1
}

func (c *cli) errf(format string, args ...any) {
	_, _ = fmt.Fprintf(c.err, "cc-login: "+format+"\n", args...)
}

// list prints the logins: the table, or format for scripts.
func (c *cli) list(ctx context.Context, format string) error {
	views, err := c.app.List(ctx)
	if err != nil {
		return err
	}

	if format != "" {
		return ui.Format(c.out, views, format)
	}

	opt := ui.Options{Now: c.app.Now()}
	if f, ok := c.out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil {
			opt.Width = w
		}
	}
	if err := ui.Table(c.out, views, opt); err != nil {
		return err
	}

	if stray := c.app.StrayCredentials(); stray != "" {
		c.errf("%s holds a sign-in that a claude run without cc-login wrote; move it into a login with: cc-login adopt NAME", stray)
	}
	return nil
}
