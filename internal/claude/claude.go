// Package claude runs the claude CLI with the credentials of one login.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// credVars are the variables that carry a login's credentials. Env drops
// them all and sets the login's own, so one login's key or token never
// reaches another login's claude.
var credVars = []string{
	"CLAUDE_SECURESTORAGE_CONFIG_DIR",
	"ANTHROPIC_API_KEY", "CC_LOGIN_API_KEY_DIR",
	"CLAUDE_CODE_OAUTH_TOKEN", "CC_LOGIN_OAUTH_TOKEN_DIR",
}

// Env is base with the credentials of one login: its directory, and for an
// API key or token login the key or token. The secret goes only into the
// environment, never into argv.
func Env(base []string, dir string, kind state.Kind) ([]string, error) {
	env := append(without(base, credVars), "CLAUDE_SECURESTORAGE_CONFIG_DIR="+dir)

	switch kind {
	case state.APIKey:
		k, err := creds.Secret(dir, kind)
		if err != nil {
			return nil, err
		}
		env = append(env, "ANTHROPIC_API_KEY="+k)

	case state.Token:
		t, err := creds.Secret(dir, kind)
		if err != nil {
			return nil, err
		}
		env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+t)

	case state.SignIn:
	}

	return env, nil
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

// ErrNotFound means there is no claude to run.
var ErrNotFound = errors.New("claude is not installed, or not in PATH")

// Find is the claude executable: $CC_LOGIN_CLAUDE, else the first claude in
// PATH that is not this program (cc-login can be installed as claude, ADR
// 0007).
func Find() (string, error) {
	if p := os.Getenv("CC_LOGIN_CLAUDE"); p != "" {
		return p, nil
	}

	self, _ := os.Executable()
	selfInfo, _ := os.Stat(self)

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		p := filepath.Join(dir, "claude")

		// PATH is the user's own setting; Find only looks at what it lists.
		fi, err := os.Stat(p) //nolint:gosec // G703: see above
		if err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
			continue
		}
		if selfInfo != nil && os.SameFile(fi, selfInfo) {
			continue
		}

		return p, nil
	}

	return "", ErrNotFound
}

// Status is what `claude auth status --json` says about a login.
type Status struct {
	SignedIn  bool
	ExpiresAt time.Time // zero when claude does not say
	Email     string
	OrgID     string
	OrgName   string
	Plan      string // subscriptionType
}

type statusJSON struct {
	LoginExpiresAt   string `json:"loginExpiresAt"`
	Email            string `json:"email"`
	OrgID            string `json:"orgId"`
	OrgName          string `json:"orgName"`
	SubscriptionType string `json:"subscriptionType"`
}

// AuthStatus runs `claude auth status --json` (the claude at path) with env.
// claude exits 0 when the login is signed in. An error means claude did not
// run at all.
func AuthStatus(ctx context.Context, path string, env []string) (Status, error) {
	cmd := exec.CommandContext(ctx, path, "auth", "status", "--json")
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return Status{}, fmt.Errorf("run %s: %w", path, err)
	}

	st := Status{SignedIn: err == nil}

	var j statusJSON
	if json.Unmarshal(out.Bytes(), &j) == nil {
		st.Email, st.OrgID, st.OrgName, st.Plan = j.Email, j.OrgID, j.OrgName, j.SubscriptionType
		if t, err := time.Parse(time.RFC3339Nano, j.LoginExpiresAt); err == nil {
			st.ExpiresAt = t
		}
	}

	return st, nil
}
