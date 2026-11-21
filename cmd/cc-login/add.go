package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/Junyi-Agent/cc-login/internal/app"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

func (c *cli) addCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add NAME [--sign-in [-- OPTS...] | --api-key | --oauth-token | --import-token]",
		Short: "Save a new login NAME",
		Long: `Save a new login NAME. Without an option, sign in with OAuth: cc-login prints a
link and opens it in the browser; choose the account, allow access, and paste
the code the page shows (CODE#STATE), shown as *. The token lasts a year, and
claude can read its account. The new login becomes the default when the
machine has none.

With --import-token, save an OAuth token you already have (sk-ant-oat...)
instead; without user:profile it has no account, plan or quota.

Without a terminal, the key, token or code is one line of stdin.`,
		Example: `  cc-login add work
  cc-login add work --sign-in -- --email me@example.com
  cc-login add ci --api-key
  cc-login add old --import-token`,
		Args: c.nameThenOpts,
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := state.Token
			if v, _ := cmd.Flags().GetBool("api-key"); v {
				kind = state.APIKey
			}
			if v, _ := cmd.Flags().GetBool("sign-in"); v {
				kind = state.SignIn
			}
			if v, _ := cmd.Flags().GetBool("import-token"); v {
				return c.addImported(cmd.Context(), args[0])
			}
			return c.add(cmd.Context(), args[0], kind, args[1:])
		},
	}

	cmd.Flags().Bool("sign-in", false, "sign in with claude auth login, which keeps the sign-in in the macOS Keychain; its options go after --")
	cmd.Flags().Bool("api-key", false, "save an Anthropic API key (sk-ant-...), shown as * while typed, and checked with the Anthropic API")
	cmd.Flags().Bool("oauth-token", false, "sign in with OAuth in the browser and save the token (the default)")
	cmd.Flags().Bool("import-token", false, "save an OAuth token you already have (sk-ant-oat...), shown as * while typed")
	cmd.MarkFlagsMutuallyExclusive("sign-in", "api-key", "oauth-token", "import-token")
	return cmd
}

func (c *cli) add(ctx context.Context, name string, kind state.Kind, args []string) error {
	res, err := c.app.Add(ctx, name, kind, args, c.prompter())
	return c.added(name, res, err)
}

// addImported saves a token the user has (--import-token).
func (c *cli) addImported(ctx context.Context, name string) error {
	res, err := c.app.ImportToken(ctx, name, c.prompter())
	return c.added(name, res, err)
}

// added tells what add saved as login name, or passes on err.
func (c *cli) added(name string, res app.Saved, err error) error {
	if err != nil {
		return err
	}

	if err := c.say("Added login %s%s.", name, account(res.Account)); err != nil {
		return err
	}
	return c.addNotes(res)
}

// addNotes say when the new login's token expires, and whether the login
// is the default. add and adopt print them.
func (c *cli) addNotes(res app.Saved) error {
	if !res.ExpiresAt.IsZero() {
		if err := c.say("The token expires about %s.", res.ExpiresAt.Local().Format("2006-01-02")); err != nil {
			return err
		}
	}

	if res.BecameDefault {
		return c.say("It is the default login.")
	}
	return c.say("Make it the default with: cc-login use %s", res.Name)
}

// account is " (me@example.com)", or "" when unknown.
func account(a string) string {
	if a == "" {
		return ""
	}
	return " (" + a + ")"
}
