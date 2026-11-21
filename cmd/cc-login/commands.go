package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/Junyi-Agent/cc-login/internal/app"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// say prints a line on stdout.
func (c *cli) say(format string, args ...any) error {
	_, err := fmt.Fprintf(c.out, format+"\n", args...)
	return err
}

func (c *cli) use(ctx context.Context, name string) error {
	if err := c.app.Use(ctx, name); err != nil {
		return err
	}
	return c.say("Default login: %s. New claude launches use it; running ones keep theirs.", name)
}

func (c *cli) rename(old, name string) error {
	if err := c.app.Rename(old, name); err != nil {
		return err
	}
	return c.say("Renamed %s to %s.", old, name)
}

func (c *cli) remove(ctx context.Context, name string, yes bool) error {
	var plan app.RemovePlan
	confirm := func(p app.RemovePlan) bool {
		plan = p
		return yes || c.confirm(fmt.Sprintf("Remove login %s?", p.Name), removeNotes(p))
	}

	if err := c.app.Remove(ctx, name, confirm); err != nil {
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

// confirm asks a yes/no question in the terminal. Without a terminal it
// says how to answer yes and answers no.
func (c *cli) confirm(title, description string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		c.errf("%s needs a terminal to ask; to answer yes without one, pass --yes", strings.TrimSuffix(title, "?"))
		return false
	}

	ok := false
	err := huh.NewConfirm().Title(title).Description(description).Affirmative("Yes").Negative("No").Value(&ok).Run()
	return err == nil && ok
}

// launch replaces cc-login with claude, run with login name ("" for the
// default) and args.
func (c *cli) launch(name string, args []string) error {
	l, err := c.app.Launch(name)
	if err != nil {
		return err
	}

	argv := append([]string{"claude"}, args...)
	if err := c.exec(l.Path, argv, l.Env); err != nil {
		return fmt.Errorf("start %s: %w", l.Path, err)
	}
	return nil // not reached: exec replaced this process
}

func (c *cli) add(ctx context.Context, name string, kind state.Kind, args []string) error {
	res, err := c.app.Add(ctx, name, kind, args, c.prompter())
	if err != nil {
		return err
	}

	if err := c.say("Added login %s%s.", name, account(res.Account)); err != nil {
		return err
	}
	return c.addNotes(res)
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

// addNotes say whether the new login is the default, and about its token.
func (c *cli) addNotes(res app.Added) error {
	if res.OverSignIn {
		if err := c.say("The token overrides the sign-in stored in %s.", res.Name); err != nil {
			return err
		}
	}
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
