package creds

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// base is an environment that carries another login's credentials, and
// the shell's own, which claude would take over a login's.
var base = []string{
	"HOME=/home/me",
	"CLAUDE_SECURESTORAGE_CONFIG_DIR=/other",
	"ANTHROPIC_API_KEY=sk-ant-other",
	"CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat-other",
	"CLAUDE_CODE_OAUTH_SCOPES=user:inference",
	"ANTHROPIC_AUTH_TOKEN=bearer-other",
	"CLAUDE_CODE_USE_BEDROCK=1",
	"CLAUDE_CODE_USE_VERTEX=1",
	"CLAUDE_CODE_USE_FOUNDRY=1",
}

func TestEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, APIKeyFile), []byte("sk-ant-mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, TokenFile), []byte("sk-ant-oat-mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	mine := []string{"HOME=/home/me", "CLAUDE_SECURESTORAGE_CONFIG_DIR=" + dir}
	cases := []struct {
		login state.Login
		want  []string
	}{
		{state.Login{Kind: state.SignIn}, mine},
		{state.Login{Kind: state.APIKey}, append(mine, "ANTHROPIC_API_KEY=sk-ant-mine")},
		{state.Login{Kind: state.Token}, append(mine, "CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat-mine")},
		{
			state.Login{Kind: state.Token, Scopes: []string{"user:inference", "user:profile"}},
			append(mine, "CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat-mine", "CLAUDE_CODE_OAUTH_SCOPES=user:inference user:profile"),
		},
	}

	for _, tc := range cases {
		got, err := Env(base, dir, &tc.login)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("Env(%s, scopes %q) = %q, %v; want %q", tc.login.Kind, tc.login.Scopes, got, err, tc.want)
		}
	}

	if base[1] != "CLAUDE_SECURESTORAGE_CONFIG_DIR=/other" {
		t.Error("Env changed its base")
	}
}

// StoredEnv keeps only the directory: no key, token or cloud provider of
// the shell or of another login.
func TestStoredEnv(t *testing.T) {
	want := []string{"HOME=/home/me", "CLAUDE_SECURESTORAGE_CONFIG_DIR=/mine"}

	if got := StoredEnv(base, "/mine"); !slices.Equal(got, want) {
		t.Errorf("StoredEnv = %q; want %q", got, want)
	}
}

// A key login whose key file is gone has no environment.
func TestEnvMissingSecret(t *testing.T) {
	got, err := Env(base, t.TempDir(), &state.Login{Kind: state.APIKey})
	if err == nil {
		t.Errorf("Env = %q; want an error", got)
	}
}
