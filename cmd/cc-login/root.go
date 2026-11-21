package main

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// rootCmd builds the cobra tree: cc-login itself, the debug log, and the
// commands, each from its own file. cobra generates the help from it.
func (c *cli) rootCmd() *cobra.Command {
	// List the commands in the order below, the order of use.
	cobra.EnableCommandSorting = false

	root := &cobra.Command{
		Use:   "cc-login",
		Short: "Several Claude Code logins on one machine",
		Long: `cc-login keeps several Claude Code logins on one machine, and the default login,
the one new claude launches use. Without a command it lists the logins.
`,
		Args: c.noCommand,
		// cobra sets this default only on its own unknown-command path.
		SuggestionsMinimumDistance: 2,
		SilenceErrors:              true,
		SilenceUsage:               true,
		RunE:                       func(cmd *cobra.Command, _ []string) error { return c.list(cmd.Context(), false) },
	}
	root.SetOut(c.out)
	root.SetErr(c.err)
	root.SetFlagErrorFunc(flagError)

	// cc-login --version, and the debug log for every command.
	root.Version = versionString()
	root.SetVersionTemplate("cc-login {{.Version}}\n")
	root.PersistentFlags().BoolP("verbose", "v", false, "log what cc-login does on stderr (also CC_LOGIN_DEBUG=1); never keys or tokens")

	// cobra runs this after the argument checks, so execute can tell a
	// wrong command line from a failed command. It checks the option groups
	// (--sign-in, --api-key, --oauth-token) only later: check them first.
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if err := cmd.ValidateFlagGroups(); err != nil {
			return err
		}
		c.ran = true

		if v, _ := cmd.Flags().GetBool("verbose"); v || os.Getenv("CC_LOGIN_DEBUG") == "1" {
			c.app.Log = slog.New(slog.NewTextHandler(c.err, &slog.HandlerOptions{Level: slog.LevelDebug}))
		}
		return nil
	}

	root.AddCommand(
		c.listCmd(), c.useCmd(), c.addCmd(), c.runCmd(), c.renewCmd(),
		c.renameCmd(), c.removeCmd(), c.adoptCmd(), c.versionCmd(),
	)
	return root
}

// noCommand rejects arguments to cc-login itself: the first one is not a
// command. The error suggests a command it looks like, or, when it names a
// login, how to make that login the default.
func (c *cli) noCommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	arg := args[0]

	if s := cmd.SuggestionsFor(arg); len(s) > 0 {
		return fmt.Errorf("unknown command %q; did you mean %s?", arg, strings.Join(s, " or "))
	}

	if slices.Contains(c.app.Names(), arg) {
		return fmt.Errorf("unknown command %q; to make login %s the default, run: cc-login use %s", arg, arg, arg)
	}

	return fmt.Errorf("unknown command %q", arg)
}
