package creds

import (
	"slices"
	"strings"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// credVars are the variables that carry a login's credentials, and the
// marks cc-login leaves on the claude it starts (CC_LOGIN_ID,
// CC_LOGIN_DEPTH). Env drops them all and sets the login's own, so one
// login's key or token never reaches another login's claude.
var credVars = []string{
	"CLAUDE_SECURESTORAGE_CONFIG_DIR",
	"ANTHROPIC_API_KEY", "CC_LOGIN_API_KEY_DIR",
	"CLAUDE_CODE_OAUTH_TOKEN", "CC_LOGIN_OAUTH_TOKEN_DIR",
	"CC_LOGIN_ID", "CC_LOGIN_DEPTH",
}

// Env is base with the credentials of one login: its directory, and for an
// API key or token login the key or token. The secret goes only into the
// environment, never into argv.
func Env(base []string, dir string, kind state.Kind) ([]string, error) {
	env := StoredEnv(base, dir)

	switch kind {
	case state.APIKey:
		k, err := Secret(dir, kind)
		if err != nil {
			return nil, err
		}
		env = append(env, "ANTHROPIC_API_KEY="+k)

	case state.Token:
		t, err := Secret(dir, kind)
		if err != nil {
			return nil, err
		}
		env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+t)

	case state.SignIn:
	}

	return env, nil
}

// StoredEnv is base with only the directory of a login: claude sees the
// sign-in stored there, not the login's key or token. remove logs that
// sign-in out.
func StoredEnv(base []string, dir string) []string {
	return append(without(base, credVars), "CLAUDE_SECURESTORAGE_CONFIG_DIR="+dir)
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
