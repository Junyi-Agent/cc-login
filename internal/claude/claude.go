// Package claude runs the claude CLI with the credentials of one login.
package claude

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Junyi-Agent/cc-login/internal/store"
)

// The variables that carry a login's credentials. Each login sets its own and
// drops the others, so the default login's key or token never leaks into
// another login's claude.
var credVars = []string{
	"CLAUDE_SECURESTORAGE_CONFIG_DIR",
	"ANTHROPIC_API_KEY", "CC_LOGIN_API_KEY_DIR",
	"CLAUDE_CODE_OAUTH_TOKEN", "CC_LOGIN_OAUTH_TOKEN_DIR",
}

// without is env without the variables in names.
func without(env []string, names []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(names, k) {
			out = append(out, kv)
		}
	}
	return out
}

// Env is base with the credentials of the login whose directory is dir: the
// directory, and its API key or token when it has one (_cc_login_with).
func Env(base []string, dir string) []string {
	env := append(without(base, credVars), "CLAUDE_SECURESTORAGE_CONFIG_DIR="+dir)

	switch store.DirKind(dir) {
	case store.KindKey:
		if k, err := store.ReadTrimmed(filepath.Join(dir, "api-key")); err == nil {
			env = append(env, "ANTHROPIC_API_KEY="+k, "CC_LOGIN_API_KEY_DIR="+dir)
		}
	case store.KindToken:
		if t, err := store.ReadTrimmed(filepath.Join(dir, "oauth-token")); err == nil {
			env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+t, "CC_LOGIN_OAUTH_TOKEN_DIR="+dir)
		}
	}

	return env
}

// StoredEnv is base with only the sign-in stored in dir, without any API key
// or token (_cc_login_stored).
func StoredEnv(base []string, dir string) []string {
	return append(without(base, credVars), "CLAUDE_SECURESTORAGE_CONFIG_DIR="+dir)
}

// Bin is the claude executable.
func Bin() string {
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	return "claude"
}

// AuthStatus runs `claude auth status --json` with env. It returns the output
// and whether claude exited 0, that is, whether the login is signed in.
func AuthStatus(ctx context.Context, env []string) ([]byte, bool) {
	cmd := exec.CommandContext(ctx, Bin(), "auth", "status", "--json")
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out

	// claude exits non-zero when the login is signed out; an error that is
	// not an exit status (no claude, say) is signed out too.
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return out.Bytes(), false
	}

	return out.Bytes(), err == nil
}

// Environ is os.Environ, kept as a variable for tests.
var Environ = os.Environ
