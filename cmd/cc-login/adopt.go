package main

import (
	"context"

	"github.com/spf13/cobra"
)

func (c *cli) adoptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adopt NAME",
		Short: "Move ~/.claude/.credentials.json into a new login NAME",
		Long: `A claude run without cc-login (an IDE extension, a plain ssh session) signs in
into ~/.claude/.credentials.json. adopt moves that file into a new login NAME;
it moves it, never copies it, because two copies of one sign-in invalidate
each other. claude sessions that read it from ~/.claude lose their
credentials. cc-login list says when there is such a file.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return c.adopt(cmd.Context(), args[0]) },
	}
}

func (c *cli) adopt(ctx context.Context, name string) error {
	src := c.app.StrayCredentials()
	res, err := c.app.Adopt(ctx, name)
	if err != nil {
		return err
	}

	if err := c.say("Moved %s into login %s%s. claude sessions that read it from there lose their credentials.", src, name, account(res.Account)); err != nil {
		return err
	}
	return c.addNotes(res)
}
