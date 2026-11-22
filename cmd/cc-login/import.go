package main

import (
	"strings"

	"github.com/spf13/cobra"
)

func (c *cli) importCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import FILE",
		Short: "Add the logins of a file of cc-login export",
		Long: `Add the logins of FILE, written by cc-login export, after asking for its
passphrase (shown as *). A login whose name this machine has is skipped:
rename or remove it, then import again. Nothing is checked online.

Without a terminal, the passphrase is a line of stdin.`,
		Example:           `  cc-login import logins.cc`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.FixedCompletions(nil, cobra.ShellCompDirectiveDefault),
		RunE:              func(_ *cobra.Command, args []string) error { return c.importFile(args[0]) },
	}
}

func (c *cli) importFile(file string) error {
	res, err := c.app.Import(file, c.prompter())
	if err != nil {
		return err
	}

	if len(res.Names) == 0 {
		if err := c.say("Imported no logins."); err != nil {
			return err
		}
	} else if err := c.say("Imported %s.", strings.Join(res.Names, ", ")); err != nil {
		return err
	}

	if len(res.Skipped) > 0 {
		if err := c.say("Skipped %s: this machine has a login of that name; rename or remove it, then import again.", strings.Join(res.Skipped, ", ")); err != nil {
			return err
		}
	}

	if res.Default != "" {
		return c.say("%s is the default login.", res.Default)
	}
	return nil
}
