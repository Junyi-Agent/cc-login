package creds

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// base is an environment that carries another login's credentials.
var base = []string{
	"HOME=/home/me",
	"CLAUDE_SECURESTORAGE_CONFIG_DIR=/other",
	"ANTHROPIC_API_KEY=sk-ant-other", "CC_LOGIN_API_KEY_DIR=/other",
	"CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat-other", "CC_LOGIN_OAUTH_TOKEN_DIR=/other",
}

func TestEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, APIKeyFile), []byte("sk-ant-mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, TokenFile), []byte("sk-ant-oat-mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		kind state.Kind
		want []string
	}{
		{state.SignIn, []string{"HOME=/home/me", "CLAUDE_SECURESTORAGE_CONFIG_DIR=" + dir}},
		{state.APIKey, []string{"HOME=/home/me", "CLAUDE_SECURESTORAGE_CONFIG_DIR=" + dir, "ANTHROPIC_API_KEY=sk-ant-mine"}},
		{state.Token, []string{"HOME=/home/me", "CLAUDE_SECURESTORAGE_CONFIG_DIR=" + dir, "CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat-mine"}},
	}

	for _, tc := range cases {
		got, err := Env(base, dir, tc.kind)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("Env(%s) = %q, %v; want %q", tc.kind, got, err, tc.want)
		}
	}

	if base[1] != "CLAUDE_SECURESTORAGE_CONFIG_DIR=/other" {
		t.Error("Env changed its base")
	}
}

// A key login whose key file is gone has no environment.
func TestEnvMissingSecret(t *testing.T) {
	got, err := Env(base, t.TempDir(), state.APIKey)
	if err == nil {
		t.Errorf("Env = %q; want an error", got)
	}
}
