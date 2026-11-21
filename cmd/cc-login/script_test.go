package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// The scripts in testdata/script run cc-login end to end, with a fake claude
// in PATH and the data root in $WORK/root.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"cc-login": func() { os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr)) },
		"claude":   fakeClaude,
	})
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			env.Setenv("CC_LOGIN_ROOT", filepath.Join(env.WorkDir, "root"))
			env.Setenv("CC_LOGIN_QUOTA_CACHE", filepath.Join(env.WorkDir, "quota.json"))
			env.Setenv("CC_LOGIN_API_URL", "http://127.0.0.1:1") // no network in tests
			env.Setenv("NO_COLOR", "1")

			// testscript links cc-login and the fake claude to one test
			// binary, and Find skips a claude that is cc-login itself (ADR
			// 0007): name the fake one. testscript puts its commands first
			// in PATH.
			bin, _, _ := strings.Cut(env.Getenv("PATH"), string(os.PathListSeparator))
			env.Setenv("CC_LOGIN_CLAUDE", filepath.Join(bin, "claude"))
			return nil
		},
	})
}

// fakeClaude answers `claude auth status --json`: signed in when the
// credentials directory holds fake-status.json (printed as the status) or
// when the environment carries a key or a token. Any other command prints
// its arguments and the credentials it got, without the secrets' values.
func fakeClaude() {
	dir := os.Getenv("CLAUDE_SECURESTORAGE_CONFIG_DIR")
	args := strings.Join(os.Args[1:], " ")

	if args == "auth status --json" {
		if b, err := os.ReadFile(filepath.Join(dir, "fake-status.json")); err == nil { //nolint:gosec // G703: a test fake
			fmt.Print(string(b))
			os.Exit(0)
		}
		if os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") != "" {
			fmt.Print("{}")
			os.Exit(0)
		}
		os.Exit(1)
	}

	if args == "auth logout" {
		_ = os.Remove(filepath.Join(dir, "fake-status.json")) //nolint:gosec // G703: a test fake
		fmt.Println("Successfully logged out")
		return
	}

	fmt.Printf("claude %s; dir %s; key %t; token %t\n", args, filepath.Base(dir),
		os.Getenv("ANTHROPIC_API_KEY") != "", os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") != "")
}
