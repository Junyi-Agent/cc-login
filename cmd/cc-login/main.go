// cc-login: several Claude Code logins on one machine, and the default login
// that claude uses. See README.md.
//
// Each command has a file here (its cobra command and what it prints) and a
// file of the same name in internal/app (what it does). root.go is the
// command tree, args.go the checks of NAME and --, prompt.go the questions
// in the terminal (ADR 0012).
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

	"github.com/Junyi-Agent/cc-login/internal/app"
)

// cli is one run of cc-login: the app, and where the output goes.
type cli struct {
	app      *app.App
	out, err io.Writer
	root     *cobra.Command
	ran      bool // the command line passed cobra's checks: set in PersistentPreRunE

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

	code := execute(ctx, args, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// execute runs the command line args and returns the exit code.
func execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	c := &cli{app: app.New(), out: stdout, err: stderr, exec: syscall.Exec}
	c.root = c.rootCmd()

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

// say prints a line on stdout.
func (c *cli) say(format string, args ...any) error {
	_, err := fmt.Fprintf(c.out, format+"\n", args...)
	return err
}

// errf prints a line on stderr.
func (c *cli) errf(format string, args ...any) {
	_, _ = fmt.Fprintf(c.err, "cc-login: "+format+"\n", args...)
}
