package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Junyi-Agent/cc-login/internal/store"
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

const (
	usageUse = "usage:\ncc-login use NAME                 make login NAME the default\n"
	usageRun = "usage:\ncc-login run NAME [-- ARGS...]    one claude launch with login NAME, without changing the default\n" +
		"                                  (ARGS: options and arguments for claude)\n"
	commands = "list use add run renew rename remove completion help"
)

// The messages and exit codes of cc-login.zsh, for command lines it rejects.
func TestRunRejects(t *testing.T) {
	setUp(t)
	usageAdd := "usage:\n" + usageText["add"] + "\n"
	usageList := "usage:\n" + usageText["list"] + "\n"
	usageRename := "usage:\n" + usageText["rename"] + "\n"
	for _, tc := range []struct {
		args   []string
		stderr string
	}{
		{[]string{"nope"}, "cc-login: unknown command nope; the commands: " + commands + "\n"},
		{[]string{"work"}, "cc-login: unknown command work; the commands: " + commands + "\n" +
			"cc-login: to make login work the default, run: cc-login use work\n"},
		{[]string{"-x", "list"}, "cc-login: unknown option -x; see cc-login help\n"},
		{[]string{"--json"}, "cc-login: unknown option --json; see cc-login help\n"},
		{[]string{"help", "nope"}, "cc-login: no command nope; the commands: " + commands + "\n"},
		{[]string{"help", "a", "b"}, "cc-login: no command a; the commands: " + commands + "\n"},
		{[]string{"help", "-h"}, "cc-login: no command -h; the commands: " + commands + "\n"},
		{[]string{"list", "extra"}, usageList},
		{[]string{"list", "--json", "x"}, usageList},
		{[]string{"use"}, usageUse},
		{[]string{"use", "-x"}, usageUse},
		{[]string{"use", ""}, usageUse},
		{[]string{"use", "a", "b"}, usageUse},
		{[]string{"rename", "a"}, usageRename},
		{[]string{"rename", "a", "-b"}, usageRename},
		{[]string{"rename", "a", ""}, usageRename},
		{[]string{"add"}, usageAdd},
		{[]string{"add", "--api-key", "a"}, usageAdd},
		{[]string{"add", "a", "--api-key", "x"}, "cc-login: add NAME --api-key takes nothing more\n" + usageAdd},
		{[]string{"add", "a", "--oauth-token", "x", "y"}, "cc-login: add NAME --oauth-token takes nothing more\n" + usageAdd},
		{[]string{"add", "a", "foo", "bar"}, "cc-login: unexpected argument foo; options for claude go after --: cc-login add a -- foo bar\n" + usageAdd},
		{[]string{"add", "a", "--sign-in", "foo"}, "cc-login: unexpected argument foo; options for claude go after --: cc-login add a --sign-in -- foo\n" + usageAdd},
		{[]string{"add", "a", "--", "--email", "x"}, "cc-login: options for claude auth login need --sign-in: cc-login add a --sign-in -- --email x\n" + usageAdd},
		// cc-login.zsh suggests --sign-in here too; run does not take it.
		{[]string{"run", "a", "foo"}, "cc-login: unexpected argument foo; options for claude go after --: cc-login run a -- foo\n" + usageRun},
		{[]string{"run"}, usageRun},
		{[]string{"completion"}, "usage:\n" + usageText["completion"] + "\n"},
		{[]string{"completion", "bash"}, "usage:\n" + usageText["completion"] + "\n"},
	} {
		code, out, errOut := runCapture(tc.args...)
		if code != 1 || out != "" || errOut != tc.stderr {
			t.Errorf("run(%q) = %d, stdout %q, stderr:\n%s\nwant 1, no stdout, stderr:\n%s", tc.args, code, out, errOut, tc.stderr)
		}
	}
}

// -h or --help before any -- prints the usage of the command and succeeds.
func TestRunHelp(t *testing.T) {
	setUp(t)
	full := "usage: cc-login COMMAND [ARGS]\n\n"
	for _, c := range cmds {
		full += usageText[c] + "\n"
	}
	for _, tc := range []struct {
		args   []string
		stdout string
	}{
		{[]string{"help"}, full},
		{[]string{"-h"}, full},
		{[]string{"--help"}, full},
		{[]string{"help", "use"}, usageUse},
		{[]string{"-h", "use"}, usageUse},
		{[]string{"use", "-h"}, usageUse},
		{[]string{"use", "a", "b", "--help"}, usageUse},
		{[]string{"run", "a", "x", "-h", "--", "y"}, usageRun},
	} {
		code, out, errOut := runCapture(tc.args...)
		if code != 0 || out != tc.stdout || errOut != "" {
			t.Errorf("run(%q) = %d, stdout:\n%s\nstderr %q; want 0 and:\n%s", tc.args, code, out, errOut, tc.stdout)
		}
	}
}

// A command line that passes the checks of a command not ported yet points to
// the zsh version and changes nothing.
func TestRunNotPorted(t *testing.T) {
	root := setUp(t)
	for _, args := range [][]string{
		{"use", "work"}, {"run", "a", "--", "-h"}, {"add", "a", "--sign-in", "--", "--email", "x"},
		{"rename", "work", "home"}, {"remove", "work"}, {"renew", "work"},
	} {
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
	if code != 1 || !strings.Contains(errOut, "not set up") {
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

// The completion offers what _cc_login_complete in cc-login.zsh offers.
func TestComplete(t *testing.T) {
	setUp(t)
	a := &app{store: store.New()}
	root := a.command()
	root.InitDefaultHelpCmd() // cobra adds the help command when it runs
	for _, tc := range []struct {
		cmd  string
		args []string
		want []string
	}{
		{"use", nil, []string{"work"}},
		{"rename", nil, []string{"work"}},
		{"rename", []string{"work"}, nil},
		{"add", nil, nil},
		{"add", []string{"a"}, []string{"--sign-in", "--api-key", "--oauth-token"}},
		{"run", []string{"work"}, []string{"--"}},
		{"run", []string{"work", "--"}, nil},
		{"help", nil, cmds},
		{"completion", nil, []string{"zsh"}},
	} {
		c, _, err := root.Find([]string{tc.cmd})
		if err != nil || c.Name() != tc.cmd {
			t.Fatalf("find %s = %v, %v", tc.cmd, c.Name(), err)
		}
		got, dir := a.complete(c, tc.args, "")
		if !slices.Equal(got, tc.want) || dir != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("complete %s %q = %q, %d; want %q", tc.cmd, tc.args, got, dir, tc.want)
		}
	}
}
