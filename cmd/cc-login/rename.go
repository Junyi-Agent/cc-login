package main

import "github.com/spf13/cobra"

func (c *cli) renameCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "rename OLD NEW",
		Short:             "Rename login OLD to NEW",
		Long:              "Rename login OLD to NEW. It stays signed in, and running claudes keep working.",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: c.completeName,
		RunE:              func(_ *cobra.Command, args []string) error { return c.rename(args[0], args[1]) },
	}
}

func (c *cli) rename(old, name string) error {
	if err := c.app.Rename(old, name); err != nil {
		return err
	}
	return c.say("Renamed %s to %s.", old, name)
}
