package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Junyi-Agent/cc-login/internal/app"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

func (c *cli) removeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove NAME",
		Short: "Sign login NAME out (claude auth logout) and delete it",
		Long: `Sign login NAME out with claude auth logout, which revokes its sign-in, and
delete it with its credentials. It asks first; without a terminal, pass --yes.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: c.completeName,
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			return c.remove(cmd.Context(), args[0], yes)
		},
	}
	cmd.Flags().BoolP("yes", "y", false, "do not ask")
	return cmd
}

func (c *cli) remove(ctx context.Context, name string, yes bool) error {
	plan, err := c.app.PlanRemove(ctx, name)
	if err != nil {
		return err
	}

	if !yes && !c.confirm(fmt.Sprintf("Remove login %s?", name), removeNotes(plan)) {
		return app.ErrCancelled
	}

	if err := c.app.Remove(ctx, plan); err != nil {
		return err
	}

	if plan.Default {
		return c.say("Removed %s. There is no default login now; choose one with: cc-login use NAME", name)
	}
	return c.say("Removed %s.", name)
}

// removeNotes say what removing the login of plan p does.
func removeNotes(p app.RemovePlan) string {
	var notes []string
	if p.LogOut {
		notes = append(notes, "claude auth logout revokes its sign-in: claude sessions running with it lose their credentials.")
	}

	switch p.Kind {
	case state.APIKey:
		notes = append(notes, "The saved API key stays valid until you revoke it in the Anthropic Console.")
	case state.Token:
		notes = append(notes, "The token stays valid until it expires: sessions using it keep working.")
	case state.SignIn:
	}

	if p.Default {
		notes = append(notes, "It is the default login: afterwards claude through cc-login starts only after cc-login use NAME.")
	}
	return strings.Join(notes, "\n")
}
