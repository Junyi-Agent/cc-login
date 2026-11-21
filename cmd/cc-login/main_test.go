package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setUp makes a data directory with login work and points CC_LOGIN_ROOT at
// it; claude and the network are never reached because no test runs list.
func setUp(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CC_LOGIN_ROOT", root)

	if err := os.MkdirAll(filepath.Join(root, ".dirs", "a1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(".dirs", "a1"), filepath.Join(root, "work")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "current"), []byte("work\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return root
}

func runCapture(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

// A wrong command line fails with the reason and a pointer to the help of
// the command, and prints nothing on stdout.
func TestRunRejects(t *testing.T) {
	setUp(t)

	cases := []struct {
		args []string
		why  string // in the error
		help string // the command whose help the error points to
	}{
		{[]string{"nope"}, `unknown command "nope"`, "cc-login"},
		{[]string{"lsit"}, "did you mean list?", "cc-login"},
		{[]string{"work"}, "to make login work the default, run: cc-login use work", "cc-login"},
		{[]string{"-x"}, "unknown shorthand flag", "cc-login"},

		{[]string{"list", "extra"}, `unknown command "extra"`, "cc-login list"},
		{[]string{"list", "--yaml"}, "unknown flag: --yaml", "cc-login list"},
		{[]string{"use"}, "accepts 1 arg(s), received 0", "cc-login use"},
		{[]string{"use", "a", "b"}, "accepts 1 arg(s), received 2", "cc-login use"},
		{[]string{"rename", "a"}, "accepts 2 arg(s), received 1", "cc-login rename"},

		{[]string{"add"}, "add takes one NAME before --, not 0 arguments", "cc-login add"},
		{[]string{"add", "a", "b"}, "add takes one NAME before --, not 2 arguments", "cc-login add"},
		{[]string{"add", "a", "--", "--email", "x"}, "options for claude auth login need --sign-in", "cc-login add"},
		{[]string{"add", "a", "--api-key", "--sign-in"}, "none of the others can be", "cc-login add"},
		{[]string{"run", "a", "foo"}, "run takes one NAME before --, not 2 arguments", "cc-login run"},
		{[]string{"renew", "--", "x"}, "renew takes one NAME before --, not 0 arguments", "cc-login renew"},
	}

	for _, tc := range cases {
		code, out, errOut := runCapture(tc.args...)
		hint := "Run '" + tc.help + " --help' for usage.\n"
		if code != 1 || out != "" || !strings.Contains(errOut, tc.why) || !strings.HasSuffix(errOut, hint) {
			t.Errorf("run(%q) = %d, stdout %q, stderr %q; want 1 and %q, then %q", tc.args, code, out, errOut, tc.why, hint)
		}
	}
}

// The arguments of a rejected command line are not echoed: a token pasted
// in the wrong place must not land in the terminal's scrollback.
func TestRunRejectsWithoutEcho(t *testing.T) {
	setUp(t)

	const pasted = "sk-ant-oat01-example"
	_, out, errOut := runCapture("add", "work", pasted)
	if strings.Contains(out+errOut, pasted) {
		t.Errorf("the error shows the argument: %q", errOut)
	}
}

// The help comes from cobra, for cc-login and for each command.
func TestRunHelp(t *testing.T) {
	setUp(t)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--help"}, "Available Commands:"},
		{[]string{"help"}, "Available Commands:"},
		{[]string{"help", "use"}, "cc-login use NAME"},
		{[]string{"use", "-h"}, "cc-login use NAME"},
		{[]string{"add", "--help"}, "--sign-in"},
		{[]string{"list", "-h"}, "--json"},
	}

	for _, tc := range cases {
		code, out, errOut := runCapture(tc.args...)
		if code != 0 || !strings.Contains(out, tc.want) || errOut != "" {
			t.Errorf("run(%q) = %d, stdout %q, stderr %q; want 0 and %q", tc.args, code, out, errOut, tc.want)
		}
	}
}

// A command line that passes the checks of a command not ported yet points to
// the zsh version and changes nothing.
func TestRunNotPorted(t *testing.T) {
	root := setUp(t)

	cases := [][]string{
		{"use", "work"},
		{"run", "a", "--", "-h"},
		{"add", "a", "--sign-in", "--", "--email", "x"},
		{"add", "--api-key", "a"},
		{"rename", "work", "home"},
		{"remove", "work"},
		{"renew", "work"},
	}

	for _, args := range cases {
		code, out, errOut := runCapture(args...)
		want := "cc-login: " + args[0] + " is not ported to Go yet; run it with the zsh cc-login\n"
		if code != 1 || out != "" || errOut != want {
			t.Errorf("run(%q) = %d, %q, %q; want 1 and %q", args, code, out, errOut, want)
		}
	}

	if _, err := os.Lstat(filepath.Join(root, "work")); err != nil {
		t.Errorf("login work is gone: %v", err)
	}
}

func TestRunNotSetUp(t *testing.T) {
	t.Setenv("CC_LOGIN_ROOT", t.TempDir())

	code, _, errOut := runCapture("list", "--json")
	if code != 1 || errOut != "cc-login: cc-login is not set up on this machine; set it up with the zsh cc-login\n" {
		t.Errorf("list on a machine that is not set up = %d, %q; want 1", code, errOut)
	}
}

func TestCompletionScript(t *testing.T) {
	setUp(t)

	code, out, _ := runCapture("completion", "zsh")
	if code != 0 || !strings.HasPrefix(out, "#compdef cc-login\n") {
		t.Errorf("completion zsh = %d, %.40q", code, out)
	}
}

// The completion offers the login names where a command takes one, and the
// options of add.
func TestComplete(t *testing.T) {
	setUp(t)

	cases := []struct {
		args []string
		want []string // among the completions
		not  []string // not among them
	}{
		{[]string{"use", ""}, []string{"work"}, nil},
		{[]string{"rename", ""}, []string{"work"}, nil},
		{[]string{"rename", "work", ""}, nil, []string{"work"}},
		{[]string{"add", ""}, nil, []string{"work"}},
		{[]string{"add", "a", "--"}, []string{"--sign-in", "--api-key", "--oauth-token"}, nil},
		{[]string{""}, []string{"list", "use", "add", "run", "renew", "rename", "remove"}, nil},
	}

	for _, tc := range cases {
		_, out, _ := runCapture(append([]string{"__complete"}, tc.args...)...)
		got := map[string]bool{}
		for _, line := range strings.Split(out, "\n") {
			name, _, _ := strings.Cut(line, "\t")
			got[name] = true
		}

		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("complete %q lacks %q:\n%s", tc.args, w, out)
			}
		}
		for _, n := range tc.not {
			if got[n] {
				t.Errorf("complete %q offers %q:\n%s", tc.args, n, out)
			}
		}
	}
}
