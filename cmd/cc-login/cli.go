package main

import (
	"strings"

	"github.com/spf13/cobra"
)

// command builds the cobra tree. cobra routes the command and generates the
// completion; the arguments are checked by parse, as in cc-login.zsh, so
// every command turns off cobra's flag parsing: cobra would accept options
// anywhere (add --api-key NAME) and print its own errors.
func (a *app) command() *cobra.Command {
	cobra.EnableCommandSorting = false // complete the commands in the usage order
	runE := func(c *cobra.Command, args []string) error { return a.dispatch(c.Name(), args) }

	root := &cobra.Command{
		Use:                "cc-login",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		SilenceErrors:      true,
		SilenceUsage:       true,
		CompletionOptions:  cobra.CompletionOptions{DisableDefaultCmd: true},
		// cc-login, cc-login -x, cc-login nope: the first argument is not a command.
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return a.dispatch("", nil)
			}
			return a.dispatch(args[0], args[1:])
		},
	}
	root.SetOut(a.out)
	root.SetErr(a.err)
	root.SetHelpFunc(func(*cobra.Command, []string) { printUsage(a.out, "") })

	for _, name := range cmds {
		c := &cobra.Command{
			Use:                name,
			Short:              short(name),
			DisableFlagParsing: true,
			RunE:               runE,
			ValidArgsFunction:  a.complete,
		}
		if name == "help" {
			root.SetHelpCommand(c)
			continue
		}
		root.AddCommand(c)
	}
	return root
}

// short is the first line of the usage of cmd, without the command line: the
// description the completion shows.
func short(cmd string) string {
	line, _, _ := strings.Cut(usageText[cmd], "\n")
	return strings.TrimSpace(line[34:])
}

// complete offers what _cc_login_complete offers: login names after the
// commands that take one, the commands after help, the options of add, and
// -- after the login of run and renew.
func (a *app) complete(c *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var out []string
	switch len(args) {
	case 0:
		switch c.Name() {
		case "use", "run", "renew", "rename", "remove":
			out = a.store.Names()
		case "help":
			out = cmds
		case "completion":
			out = []string{"zsh"}
		}
	case 1:
		switch c.Name() {
		case "add":
			out = []string{"--sign-in", "--api-key", "--oauth-token"}
		case "run", "renew":
			out = []string{"--"}
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
