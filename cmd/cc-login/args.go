package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// beforeDash is how many arguments come before --.
func beforeDash(cmd *cobra.Command, args []string) int {
	if dash := cmd.ArgsLenAtDash(); dash != -1 {
		return dash
	}
	return len(args)
}

// nameThenOpts checks add and renew: one NAME, then optionally -- and
// options for claude. add takes them only with --sign-in.
func (c *cli) nameThenOpts(cmd *cobra.Command, args []string) error {
	dash := beforeDash(cmd, args)

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
	if dash := beforeDash(cmd, args); dash > 1 {
		return fmt.Errorf("%s takes at most one NAME before --, not %d arguments", cmd.Name(), dash)
	}
	return nil
}

// splitDash splits the arguments of run into the NAME ("" for none) and
// the arguments for claude after --.
func splitDash(cmd *cobra.Command, args []string) (string, []string) {
	dash := beforeDash(cmd, args)

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

// flagError adds to an unknown option of run, add or renew that options for
// claude go after --.
func flagError(cmd *cobra.Command, err error) error {
	if !strings.HasPrefix(err.Error(), "unknown") { // unknown flag, unknown shorthand flag
		return err
	}

	switch cmd.Name() {
	case "run":
		return fmt.Errorf("%w; options for claude go after --: cc-login run [NAME] -- OPTS", err)
	case "renew":
		return fmt.Errorf("%w; options for claude auth login go after --: cc-login renew NAME -- OPTS", err)
	case "add":
		return fmt.Errorf("%w; options for claude auth login go after --sign-in --: cc-login add NAME --sign-in -- OPTS", err)
	}
	return err
}
