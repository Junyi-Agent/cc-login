package main

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// command builds the cobra tree: the commands, their options, the argument
// checks and the completion. cobra generates the help from it.
func (c *cli) command() *cobra.Command {
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
		RunE:                       c.runs(func(cmd *cobra.Command, _ []string) error { return c.list(cmd.Context(), false) }),
	}
	root.SetOut(c.out)
	root.SetErr(c.err)

	// cc-login --version, and the debug log for every command.
	root.Version = versionString()
	root.SetVersionTemplate("cc-login {{.Version}}\n")
	root.PersistentFlags().BoolP("verbose", "v", false, "log what cc-login does on stderr (also CC_LOGIN_DEBUG=1); never keys or tokens")
	root.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		if v, _ := cmd.Flags().GetBool("verbose"); v || os.Getenv("CC_LOGIN_DEBUG") == "1" {
			c.app.Log = slog.New(slog.NewTextHandler(c.err, &slog.HandlerOptions{Level: slog.LevelDebug}))
		}
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the logins; ▸ marks the default",
		Args:  cobra.NoArgs,
		RunE: c.runs(func(cmd *cobra.Command, _ []string) error {
			asJSON, _ := cmd.Flags().GetBool("json")
			return c.list(cmd.Context(), asJSON)
		}),
	}
	list.Flags().Bool("json", false, "print JSON, for scripts")

	use := &cobra.Command{
		Use:               "use NAME",
		Short:             "Make login NAME the default",
		Long:              "Make login NAME the default: the login of claude through cc-login. Running claudes keep theirs.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: c.completeName,
		RunE:              c.runs(func(cmd *cobra.Command, args []string) error { return c.use(cmd.Context(), args[0]) }),
	}

	add := &cobra.Command{
		Use:   "add NAME [--sign-in [-- OPTS...] | --api-key | --oauth-token]",
		Short: "Save a new login NAME",
		Long: `Save a new login NAME. Without an option, read a token from claude setup-token
(sk-ant-oat...), shown as *, and check it with the Anthropic API; it lasts a
year. The new login becomes the default when the machine has none.

Without a terminal, the key or token is one line of stdin.`,
		Example: `  cc-login add work
  cc-login add work --sign-in -- --email me@example.com
  cc-login add ci --api-key`,
		Args: c.nameThenOpts,
		RunE: c.runs(func(cmd *cobra.Command, args []string) error {
			kind := state.Token
			if v, _ := cmd.Flags().GetBool("api-key"); v {
				kind = state.APIKey
			}
			if v, _ := cmd.Flags().GetBool("sign-in"); v {
				kind = state.SignIn
			}
			return c.add(cmd.Context(), args[0], kind, args[1:])
		}),
	}
	add.Flags().Bool("sign-in", false, "sign in with claude auth login, which keeps the sign-in in the macOS Keychain; its options go after --")
	add.Flags().Bool("api-key", false, "save an Anthropic API key (sk-ant-...), shown as * while typed, and checked with the Anthropic API")
	add.Flags().Bool("oauth-token", false, "save a token from claude setup-token (the default)")
	add.MarkFlagsMutuallyExclusive("sign-in", "api-key", "oauth-token")

	run := &cobra.Command{
		Use:   "run [NAME] [-- ARGS...]",
		Short: "Start claude with login NAME, or with the default login",
		Long: `Start claude with login NAME: cc-login replaces itself with claude, whose
environment holds that login's credentials and no other's. ARGS go to claude.

Without NAME: the login of the claude this runs in (a claude started by
cc-login marks its children with CC_LOGIN_ID), else the default login. A link
named claude to cc-login, early in PATH, does the same as cc-login run --.`,
		Example: `  cc-login run work
  cc-login run -- --continue
  cc-login run ci -- -p "summarize the diff"`,
		Args:              c.optionalNameThenOpts,
		ValidArgsFunction: c.completeName,
		RunE: c.runs(func(cmd *cobra.Command, args []string) error {
			name, rest := splitDash(cmd, args)
			return c.launch(name, rest)
		}),
	}

	renew := &cobra.Command{
		Use:   "renew NAME [-- OPTS...]",
		Short: "Sign login NAME in again, so that it expires later",
		Long: `Sign login NAME in again with claude auth login, also while it is signed in, so
that it expires later; OPTS are options for claude auth login. For a token
login, save a new token from claude setup-token.`,
		Args:              c.nameThenOpts,
		ValidArgsFunction: c.completeName,
		RunE: c.runs(func(cmd *cobra.Command, args []string) error {
			return c.renew(cmd.Context(), args[0], args[1:])
		}),
	}

	rename := &cobra.Command{
		Use:               "rename OLD NEW",
		Short:             "Rename login OLD to NEW",
		Long:              "Rename login OLD to NEW. It stays signed in, and running claudes keep working.",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: c.completeName,
		RunE:              c.runs(func(_ *cobra.Command, args []string) error { return c.rename(args[0], args[1]) }),
	}

	remove := &cobra.Command{
		Use:   "remove NAME",
		Short: "Sign login NAME out (claude auth logout) and delete it",
		Long: `Sign login NAME out with claude auth logout, which revokes its sign-in, and
delete it with its credentials. It asks first; without a terminal, pass --yes.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: c.completeName,
		RunE: c.runs(func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			return c.remove(cmd.Context(), args[0], yes)
		}),
	}
	remove.Flags().BoolP("yes", "y", false, "do not ask")

	adopt := &cobra.Command{
		Use:   "adopt NAME",
		Short: "Move ~/.claude/.credentials.json into a new login NAME",
		Long: `A claude run without cc-login (an IDE extension, a plain ssh session) signs in
into ~/.claude/.credentials.json. adopt moves that file into a new login NAME;
it moves it, never copies it, because two copies of one sign-in invalidate
each other. claude sessions that read it from ~/.claude lose their
credentials. cc-login list says when there is such a file.`,
		Args: cobra.ExactArgs(1),
		RunE: c.runs(func(cmd *cobra.Command, args []string) error { return c.adopt(cmd.Context(), args[0]) }),
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of cc-login",
		Args:  cobra.NoArgs,
		RunE: c.runs(func(*cobra.Command, []string) error {
			return c.say("cc-login %s (%s)", versionString(), goVersion())
		}),
	}

	root.AddCommand(list, use, add, run, renew, rename, remove, adopt, versionCmd)
	return root
}

// runs marks that the command line passed cobra's checks before fn runs, so
// run can tell a wrong command line from a failed command.
func (c *cli) runs(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		c.ran = true
		return fn(cmd, args)
	}
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

// nameThenOpts checks add, run and renew: one NAME, then optionally -- and
// options for claude. add takes them only with --sign-in.
func (c *cli) nameThenOpts(cmd *cobra.Command, args []string) error {
	dash := cmd.ArgsLenAtDash()
	if dash == -1 {
		dash = len(args)
	}

	// The arguments are not echoed: a token pasted by mistake would land in
	// the terminal's scrollback.
	if dash != 1 {
		return fmt.Errorf("%s takes one NAME before --, not %d arguments", cmd.Name(), dash)
	}

	signIn, _ := cmd.Flags().GetBool("sign-in")
	if cmd.Name() == "add" && len(args) > 1 && !signIn {
		return fmt.Errorf("options for claude auth login need --sign-in: cc-login add %s --sign-in -- OPTS", args[0])
	}

	return nil
}

// optionalNameThenOpts checks run: at most a NAME, then optionally -- and
// options for claude.
func (c *cli) optionalNameThenOpts(cmd *cobra.Command, args []string) error {
	dash := cmd.ArgsLenAtDash()
	if dash == -1 {
		dash = len(args)
	}

	if dash > 1 {
		return fmt.Errorf("%s takes at most one NAME before --, not %d arguments", cmd.Name(), dash)
	}
	return nil
}

// splitDash splits the arguments of run into the NAME ("" for none) and
// the arguments for claude after --.
func splitDash(cmd *cobra.Command, args []string) (string, []string) {
	dash := cmd.ArgsLenAtDash()
	if dash == -1 {
		dash = len(args)
	}

	name := ""
	if dash == 1 {
		name = args[0]
	}
	return name, args[dash:]
}

// completeName completes a login name as the first argument.
func (c *cli) completeName(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return c.app.Names(), cobra.ShellCompDirectiveNoFileComp
}
