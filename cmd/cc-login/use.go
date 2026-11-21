package main

import (
	"context"

	"github.com/spf13/cobra"
)

func (c *cli) useCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "use NAME",
		Short:             "Make login NAME the default",
		Long:              "Make login NAME the default: the login of claude through cc-login. Running claudes keep theirs.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: c.completeName,
		RunE:              func(cmd *cobra.Command, args []string) error { return c.use(cmd.Context(), args[0]) },
	}
}

func (c *cli) use(ctx context.Context, name string) error {
	if err := c.app.Use(ctx, name); err != nil {
		return err
	}
	return c.say("Default login: %s. New claude launches use it; running ones keep theirs.", name)
}
