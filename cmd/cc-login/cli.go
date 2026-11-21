package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// command builds the cobra tree: the commands, their options, the argument
// checks and the completion. cobra generates the help from it.
func (a *app) command() *cobra.Command {
	// List the commands in the order below, the order of use.
	cobra.EnableCommandSorting = false

	root := &cobra.Command{
		Use:   "cc-login",
		Short: "Several Claude Code logins on one machine",
		Long: `cc-login keeps several Claude Code logins on one machine, and the default login,
the one new claude launches use. Without a command it lists the logins.

This Go port runs list; the other commands still run in cc-login.zsh.`,
		Args: a.noCommand,
		// cobra sets this default only on its own unknown-command path.
		SuggestionsMinimumDistance: 2,
		SilenceErrors:              true,
		SilenceUsage:               true,
		RunE:                       a.runs(func(*cobra.Command, []string) error { return a.list(false) }),
	}
	root.SetOut(a.out)
	root.SetErr(a.err)

	list := &cobra.Command{
		Use:   "list",
		Short: "List the logins; ▸ marks the default",
		Args:  cobra.NoArgs,
		RunE: a.runs(func(c *cobra.Command, _ []string) error {
			asJSON, _ := c.Flags().GetBool("json")
			return a.list(asJSON)
		}),
	}
	list.Flags().Bool("json", false, "print a JSON array, one object per login")

	use := &cobra.Command{
		Use:               "use NAME",
		Short:             "Make login NAME the default",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: a.completeName,
		RunE:              a.runs(a.notPorted),
	}

	add := &cobra.Command{
		Use:   "add NAME [--sign-in [-- OPTS...] | --api-key | --oauth-token]",
		Short: "Save a new login, or a token for login NAME",
		Long: `Save login NAME. Without an option, read a token from claude setup-token
(sk-ant-oat...) without echo and check it with the Anthropic API; it lasts a
year. For an existing login NAME the token overrides its stored sign-in.
A new login becomes the default when the machine has none.`,
		Example: `  cc-login add work
  cc-login add work --sign-in -- --email me@example.com
  cc-login add ci --api-key`,
		Args: a.nameThenOpts,
		RunE: a.runs(a.notPorted),
	}
	add.Flags().Bool("sign-in", false, "sign in with claude auth login, which keeps the sign-in in the macOS Keychain; its options go after --")
	add.Flags().Bool("api-key", false, "save an Anthropic API key (sk-ant-...), read without echo and checked with the Anthropic API")
	add.Flags().Bool("oauth-token", false, "save a token from claude setup-token (the default)")
	add.MarkFlagsMutuallyExclusive("sign-in", "api-key", "oauth-token")

	run := &cobra.Command{
		Use:               "run NAME [-- ARGS...]",
		Short:             "Launch claude once with login NAME, without changing the default",
		Args:              a.nameThenOpts,
		ValidArgsFunction: a.completeName,
		RunE:              a.runs(a.notPorted),
	}

	renew := &cobra.Command{
		Use:   "renew NAME [-- OPTS...]",
		Short: "Sign login NAME in again, so that it expires later",
		Long: `Sign login NAME in again with claude auth login, also while it is signed in, so
that it expires later; OPTS are options for claude auth login. For a token
login, save a new token from claude setup-token.`,
		Args:              a.nameThenOpts,
		ValidArgsFunction: a.completeName,
		RunE:              a.runs(a.notPorted),
	}

	rename := &cobra.Command{
		Use:               "rename OLD NEW",
		Short:             "Rename login OLD to NEW",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: a.completeName,
		RunE:              a.runs(a.notPorted),
	}

	remove := &cobra.Command{
		Use:               "remove NAME",
		Short:             "Sign login NAME out (claude auth logout) and delete it",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: a.completeName,
		RunE:              a.runs(a.notPorted),
	}

	root.AddCommand(list, use, add, run, renew, rename, remove)
	return root
}

// runs marks that the command line passed cobra's checks before fn runs, so
// run can tell a wrong command line from a failed command.
func (a *app) runs(fn func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(c *cobra.Command, args []string) error {
		a.ran = true
		return fn(c, args)
	}
}

// noCommand rejects arguments to cc-login itself: the first one is not a
// command. The error suggests a command it looks like, or, when it names a
// login, how to make that login the default.
func (a *app) noCommand(c *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	arg := args[0]

	if s := c.SuggestionsFor(arg); len(s) > 0 {
		return fmt.Errorf("unknown command %q; did you mean %s?", arg, strings.Join(s, " or "))
	}

	if _, ok := a.store.Dir(arg); ok {
		return fmt.Errorf("unknown command %q; to make login %s the default, run: cc-login use %s", arg, arg, arg)
	}

	return fmt.Errorf("unknown command %q", arg)
}

// nameThenOpts checks add, run and renew: one NAME, then optionally -- and
// options for claude. add takes them only with --sign-in.
func (a *app) nameThenOpts(c *cobra.Command, args []string) error {
	dash := c.ArgsLenAtDash()
	if dash == -1 {
		dash = len(args)
	}

	// The arguments are not echoed: a token pasted by mistake would land in
	// the terminal's scrollback.
	if dash != 1 {
		return fmt.Errorf("%s takes one NAME before --, not %d arguments", c.Name(), dash)
	}

	signIn, _ := c.Flags().GetBool("sign-in")
	if c.Name() == "add" && len(args) > 1 && !signIn {
		return fmt.Errorf("options for claude auth login need --sign-in: cc-login add %s --sign-in -- OPTS", args[0])
	}

	return nil
}

func (a *app) notPorted(c *cobra.Command, _ []string) error {
	return fmt.Errorf("%s is not ported to Go yet; run it with the zsh cc-login", c.Name())
}

// completeName completes a login name as the first argument.
func (a *app) completeName(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return a.store.Names(), cobra.ShellCompDirectiveNoFileComp
}
