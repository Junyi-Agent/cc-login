package main

import (
	"strings"

	"github.com/spf13/cobra"
)

func (c *cli) exportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export FILE [NAME...]",
		Short: "Write token and API key logins into FILE, encrypted with a passphrase",
		Long: `Write the token and API key logins named, or all of them, into a new file FILE,
encrypted with a passphrase you type twice (shown as *). cc-login import FILE
adds them on another machine. A sign-in login stays on this machine: add it
on the other one with cc-login add NAME --sign-in.

Without a terminal, the passphrase is a line of stdin, twice.`,
		Example: `  cc-login export logins.cc
  cc-login export work.cc work ci`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: c.completeFileThenNames,
		RunE:              func(_ *cobra.Command, args []string) error { return c.export(args[0], args[1:]) },
	}
}

func (c *cli) export(file string, names []string) error {
	res, err := c.app.Export(file, names, c.prompter())
	if err != nil {
		return err
	}

	if err := c.say("Exported %s to %s.", strings.Join(res.Names, ", "), file); err != nil {
		return err
	}
	if len(res.SignIns) > 0 {
		if err := c.say("Left out sign-in logins %s: on the other machine, run cc-login add NAME --sign-in.", strings.Join(res.SignIns, ", ")); err != nil {
			return err
		}
	}
	return c.say("Anyone with %s and the passphrase can use these logins: delete it after the import.", file)
}

// completeFileThenNames completes a file, then login names.
func (c *cli) completeFileThenNames(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return nil, cobra.ShellCompDirectiveDefault
	}
	return c.app.Names(), cobra.ShellCompDirectiveNoFileComp
}
