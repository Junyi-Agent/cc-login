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
