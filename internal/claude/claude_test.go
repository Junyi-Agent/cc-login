package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// executable writes an executable script at path.
func executable(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestFind(t *testing.T) {
	notExec, dirOnly, good := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(notExec, "claude"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dirOnly, "claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	executable(t, filepath.Join(good, "claude"), "exit 0\n")

	t.Setenv("CC_LOGIN_CLAUDE", "")
	t.Setenv("PATH", strings.Join([]string{notExec, dirOnly, good}, string(os.PathListSeparator)))
	if p, err := Find(); err != nil || p != filepath.Join(good, "claude") {
		t.Errorf("Find = %q, %v; want the executable in %s", p, err, good)
	}

	t.Setenv("PATH", strings.Join([]string{notExec, dirOnly}, string(os.PathListSeparator)))
	if p, err := Find(); !errors.Is(err, ErrNotFound) {
		t.Errorf("Find = %q, %v; want ErrNotFound", p, err)
	}

	// A claude that is this program (cc-login installed as claude) is skipped.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	link := t.TempDir()
	if err := os.Symlink(self, filepath.Join(link, "claude")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", strings.Join([]string{link, good}, string(os.PathListSeparator)))
	if p, err := Find(); err != nil || p != filepath.Join(good, "claude") {
		t.Errorf("Find = %q, %v; want to skip the link to itself", p, err)
	}

	// InPath is what a plain claude runs: the link, which is this program.
	if p, linked := InPath(); p != filepath.Join(link, "claude") || !linked {
		t.Errorf("InPath = %q, %t; want the link", p, linked)
	}
	t.Setenv("PATH", strings.Join([]string{good, link}, string(os.PathListSeparator)))
	if p, linked := InPath(); p != filepath.Join(good, "claude") || linked {
		t.Errorf("InPath = %q, %t; want the claude before the link", p, linked)
	}
	t.Setenv("PATH", notExec)
	if p, linked := InPath(); p != "" || linked {
		t.Errorf("InPath = %q, %t; want none", p, linked)
	}

	t.Setenv("CC_LOGIN_CLAUDE", "/opt/claude")
	if p, err := Find(); err != nil || p != "/opt/claude" {
		t.Errorf("Find = %q, %v; want CC_LOGIN_CLAUDE", p, err)
	}
}

func TestAuthStatus(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in"), filepath.Join(dir, "out")
	executable(t, in, `test "$1 $2 $3" = "auth status --json" || exit 9
echo '{"loggedIn":true,"email":"me@example.com","orgId":"org-1","orgName":"Example","subscriptionType":"max","loginExpiresAt":"2026-12-01T10:00:00.123Z"}'
`)
	executable(t, out, "echo '{\"loggedIn\":false}'\nexit 1\n")
	ctx := context.Background()

	st, err := AuthStatus(ctx, in, nil)
	want := AuthInfo{
		SignedIn: true, Email: "me@example.com", Plan: "max",
		ExpiresAt: time.Date(2026, 12, 1, 10, 0, 0, 123_000_000, time.UTC),
	}
	if err != nil || !st.ExpiresAt.Equal(want.ExpiresAt) || st.SignedIn != want.SignedIn ||
		st.Email != want.Email || st.Plan != want.Plan {
		t.Errorf("signed in: %+v, %v; want %+v", st, err, want)
	}

	st, err = AuthStatus(ctx, out, nil)
	if err != nil || st.SignedIn {
		t.Errorf("signed out: %+v, %v; want not signed in, no error", st, err)
	}

	if _, err := AuthStatus(ctx, filepath.Join(dir, "missing"), nil); err == nil {
		t.Error("missing claude: want an error")
	}
}

// claude gets exactly the environment it is given.
func TestAuthStatusEnv(t *testing.T) {
	p := filepath.Join(t.TempDir(), "claude")
	executable(t, p, `test "$CLAUDE_SECURESTORAGE_CONFIG_DIR" = /mine && test -z "$ANTHROPIC_API_KEY"`+"\n")

	st, err := AuthStatus(context.Background(), p, []string{"CLAUDE_SECURESTORAGE_CONFIG_DIR=/mine"})
	if err != nil || !st.SignedIn {
		t.Errorf("AuthStatus = %+v, %v; want the env to reach claude", st, err)
	}
}
