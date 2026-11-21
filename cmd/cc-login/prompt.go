package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

// prompter is app.Prompter for the command line: it asks with huh on a
// terminal; without one it reads lines of stdin (a key or a token piped in)
// and asks nothing else.
type prompter struct {
	c  *cli
	in *bufio.Reader
}

func (c *cli) prompter() *prompter { return &prompter{c: c, in: bufio.NewReader(os.Stdin)} }

// terminal reports whether stdin is a terminal, so cc-login can ask.
func terminal() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// Secret reads a key or a token, shown as * on a terminal.
func (p *prompter) Secret(title, help string, valid func(string) error) (string, error) {
	if !terminal() {
		line, err := p.in.ReadString('\n')
		if line = strings.TrimSpace(line); line == "" {
			if errors.Is(err, io.EOF) || err == nil {
				err = errors.New("no input: pipe the value into stdin, or run in a terminal")
			}
			return "", err
		}
		return line, nil
	}

	var v string
	err := secretInput(title, help, valid, &v).Run()
	return strings.TrimSpace(v), err
}

// secretInput is the input box for a key or a token: each character typed
// or pasted shows as *, so the user sees that the paste arrived and how long
// it is, but not the value.
func secretInput(title, help string, valid func(string) error, v *string) *huh.Input {
	return huh.NewInput().Title(title).Description(help).
		EchoMode(huh.EchoModePassword).
		Validate(func(s string) error { return valid(strings.TrimSpace(s)) }).
		Value(v)
}

// Line asks a question whose answer may show; "" without a terminal.
func (p *prompter) Line(title, help string) (string, error) {
	if !terminal() {
		return "", nil
	}

	var v string
	err := huh.NewInput().Title(title).Description(help).Value(&v).Run()
	return strings.TrimSpace(v), err
}

// Terminal says whether stdin is a terminal.
func (p *prompter) Terminal() bool { return terminal() }

// Note is a line for the user on stderr.
func (p *prompter) Note(msg string) {
	_, _ = fmt.Fprintln(p.c.err, msg)
}

// confirm asks a yes/no question in the terminal. Without a terminal it
// says how to answer yes and answers no.
func (c *cli) confirm(title, description string) bool {
	if !terminal() {
		c.errf("%s needs a terminal to ask; to answer yes without one, pass --yes", strings.TrimSuffix(title, "?"))
		return false
	}

	ok := false
	err := huh.NewConfirm().Title(title).Description(description).Affirmative("Yes").Negative("No").Value(&ok).Run()
	return err == nil && ok
}
