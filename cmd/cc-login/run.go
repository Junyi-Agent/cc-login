package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (c *cli) runCmd() *cobra.Command {
	return &cobra.Command{
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
		RunE: func(cmd *cobra.Command, args []string) error {
			name, rest := splitDash(cmd, args)
			return c.run(name, rest)
		},
	}
}

// run replaces cc-login with claude, run with login name ("" for the
// default) and args.
func (c *cli) run(name string, args []string) error {
	x, err := c.app.Run(name)
	if err != nil {
		return err
	}

	argv := append([]string{"claude"}, args...)
	if err := c.exec(x.Path, argv, x.Env); err != nil {
		return fmt.Errorf("start %s: %w", x.Path, err)
	}
	return nil // not reached: exec replaced this process
}
