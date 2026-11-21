package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/Junyi-Agent/cc-login/internal/ui"
)

func (c *cli) listCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the logins; ▸ marks the default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			asJSON, _ := cmd.Flags().GetBool("json")
			return c.list(cmd.Context(), asJSON)
		},
	}
	cmd.Flags().Bool("json", false, "print JSON, for scripts")
	return cmd
}

// list prints the logins: the table, or JSON for scripts.
func (c *cli) list(ctx context.Context, asJSON bool) error {
	views, err := c.app.List(ctx)
	if err != nil {
		return err
	}

	if asJSON {
		return ui.JSON(c.out, views)
	}

	if err := ui.Table(c.out, views, c.app.Now()); err != nil {
		return err
	}

	if stray := c.app.StrayCredentials(); stray != "" {
		c.errf("%s holds a sign-in that a claude run without cc-login wrote; move it into a login with: cc-login adopt NAME", stray)
	}
	return nil
}
