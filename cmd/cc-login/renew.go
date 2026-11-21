package main

import (
	"context"

	"github.com/spf13/cobra"
)

func (c *cli) renewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "renew NAME [-- OPTS...]",
		Short: "Sign login NAME in again, so that it expires later",
		Long: `Sign login NAME in again with claude auth login, also while it is signed in, so
that it expires later; OPTS are options for claude auth login. For a token
login, run claude setup-token and save the new token it prints.`,
		Args:              c.nameThenOpts,
		ValidArgsFunction: c.completeName,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.renew(cmd.Context(), args[0], args[1:])
		},
	}
}

func (c *cli) renew(ctx context.Context, name string, args []string) error {
	res, err := c.app.Renew(ctx, name, args, c.prompter())
	if err != nil {
		return err
	}

	if res.ExpiresAt.IsZero() {
		return c.say("Renewed login %s.", name)
	}
	return c.say("Renewed login %s; it expires %s.", name, res.ExpiresAt.Local().Format("2006-01-02 15:04"))
}
