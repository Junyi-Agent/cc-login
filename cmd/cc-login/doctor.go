package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Junyi-Agent/cc-login/internal/app"
)

func (c *cli) doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the setup: claude, the claude link, logins",
		Long: `Check the setup, offline: the claude cc-login runs, the claude link, keys and
tokens set in the shell, state.json, and each login's files and expiry. Each
problem comes with what to do. It exits 1 when something does not work;
warnings alone exit 0.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return c.doctor() },
	}
}

// doctor prints the findings of the checks, one line each, the fix under
// the ones that are not OK.
func (c *cli) doctor() error {
	fs := c.app.Doctor()

	marks := map[app.Level]string{app.OK: "✓", app.Warn: "!", app.Fail: "✗"}
	for _, f := range fs {
		if err := c.print("%s %s", marks[f.Level], f.What); err != nil {
			return err
		}

		if f.Fix != "" {
			if err := c.print("    fix: %s", f.Fix); err != nil {
				return err
			}
		}
	}

	if n := app.Problems(fs); n > 0 {
		return fmt.Errorf("%d problem(s) found", n)
	}
	return nil
}
