package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// has reports whether a finding of that level mentions text.
func has(fs []Finding, level Level, text string) bool {
	for _, f := range fs {
		if f.Level == level && strings.Contains(f.What, text) {
			return true
		}
	}
	return false
}

// healthy is an App where doctor finds nothing: the link, a default login,
// a sign-in, a token that reads its profile, a key. f is its fake claude.
func healthy(t *testing.T) (a *App, f *fake) {
	t.Helper()
	exp := now.Add(300 * 24 * time.Hour)
	f = &fake{status: map[string]claude.AuthInfo{}, linked: true}

	a = newApp(t, map[string]*state.Login{
		"a1": {Name: "work", Kind: state.SignIn},
		"t1": {Name: "ci", Kind: state.Token, TokenExpiresAt: &exp, Scopes: []string{"user:inference", "user:profile"}},
		"k1": {Name: "api", Kind: state.APIKey},
	}, "a1", f)
	return a, f
}

func TestDoctorHealthy(t *testing.T) {
	a, _ := healthy(t)
	a.Env = []string{"PATH=/bin"}
	a.Env = append(a.Env, "HOME="+t.TempDir(), "CLAUDE_CONFIG_DIR="+t.TempDir())

	fs := a.Doctor()
	for _, f := range fs {
		if f.Level != OK || f.Fix != "" {
			t.Errorf("finding %+v; want only OK", f)
		}
	}
	if Problems(fs) != 0 || !has(fs, OK, "default login: work") || !has(fs, OK, "login ci: ok") {
		t.Errorf("findings = %+v", fs)
	}
}

func TestDoctorFindsProblems(t *testing.T) {
	past := now.Add(-time.Hour)
	soon := now.Add(48 * time.Hour)

	cases := []struct {
		name  string
		setup func(t *testing.T, a *App, f *fake)
		level Level
		text  string
	}{
		{"no link", func(_ *testing.T, _ *App, f *fake) { f.linked = false }, Fail, "without cc-login"},
		{"no claude", func(_ *testing.T, _ *App, f *fake) { f.pathErr = claude.ErrNotFound }, Fail, "claude: claude is not installed"},
		{"shell key", func(_ *testing.T, a *App, _ *fake) { a.Env = append(a.Env, "ANTHROPIC_API_KEY=sk-ant-x") }, Warn, "ANTHROPIC_API_KEY is set"},
		{"shell token", func(_ *testing.T, a *App, _ *fake) { a.Env = append(a.Env, "CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat-x") }, Warn, "CLAUDE_CODE_OAUTH_TOKEN is set"},
		{"shell auth token", func(_ *testing.T, a *App, _ *fake) { a.Env = append(a.Env, "ANTHROPIC_AUTH_TOKEN=x") }, Warn, "ANTHROPIC_AUTH_TOKEN is set"},
		{"shell bedrock", func(_ *testing.T, a *App, _ *fake) { a.Env = append(a.Env, "CLAUDE_CODE_USE_BEDROCK=1") }, Warn, "CLAUDE_CODE_USE_BEDROCK is set"},
		{"shell vertex", func(_ *testing.T, a *App, _ *fake) { a.Env = append(a.Env, "CLAUDE_CODE_USE_VERTEX=1") }, Warn, "CLAUDE_CODE_USE_VERTEX is set"},
		{"shell foundry", func(_ *testing.T, a *App, _ *fake) { a.Env = append(a.Env, "CLAUDE_CODE_USE_FOUNDRY=1") }, Warn, "CLAUDE_CODE_USE_FOUNDRY is set"},
		{"no default", func(t *testing.T, a *App, _ *fake) {
			if err := a.Store.Update(func(st *state.State) error { st.Default = ""; return nil }); err != nil {
				t.Fatal(err)
			}
		}, Fail, "no default login"},
		{"token file gone", func(t *testing.T, a *App, _ *fake) {
			if err := os.Remove(filepath.Join(a.Store.Dir("t1"), creds.TokenFile)); err != nil {
				t.Fatal(err)
			}
		}, Fail, "login ci: oauth-token is gone"},
		{"key readable", func(t *testing.T, a *App, _ *fake) {
			if err := os.Chmod(filepath.Join(a.Store.Dir("k1"), creds.APIKeyFile), 0o644); err != nil { //nolint:gosec // G302: the test makes a file others can read
				t.Fatal(err)
			}
		}, Warn, "login api: api-key is 0644"},
		{"directory open", func(t *testing.T, a *App, _ *fake) {
			if err := os.Chmod(a.Store.Dir("a1"), 0o755); err != nil { //nolint:gosec // G302: the test opens a directory to others
				t.Fatal(err)
			}
		}, Warn, "login work: its directory is 0755"},
		{"directory gone", func(t *testing.T, a *App, _ *fake) {
			if err := os.RemoveAll(a.Store.Dir("a1")); err != nil {
				t.Fatal(err)
			}
		}, Fail, "login work: its credentials directory is gone"},
		{"token expired", func(t *testing.T, a *App, _ *fake) {
			setLogin(t, a, "t1", func(l *state.Login) { l.TokenExpiresAt = &past })
		}, Fail, "login ci: its token has expired"},
		{"token expiring", func(t *testing.T, a *App, _ *fake) {
			setLogin(t, a, "t1", func(l *state.Login) { l.TokenExpiresAt = &soon })
		}, Warn, "login ci: its token expires within a week"},
		{"old token", func(t *testing.T, a *App, _ *fake) { setLogin(t, a, "t1", func(l *state.Login) { l.Scopes = nil }) }, Warn, "no user:profile"},
		{"state.json broken", func(t *testing.T, a *App, _ *fake) {
			if err := os.WriteFile(filepath.Join(a.Store.Root, "state.json"), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, Fail, "state.json:"},
	}

	for _, tc := range cases {
		a, f := healthy(t)
		a.Env = []string{"PATH=/bin", "HOME=" + t.TempDir(), "CLAUDE_CONFIG_DIR=" + t.TempDir()}
		tc.setup(t, a, f)

		fs := a.Doctor()
		if !has(fs, tc.level, tc.text) {
			t.Errorf("%s: findings %+v; want %v with %q", tc.name, fs, tc.level, tc.text)
		}

		for _, x := range fs {
			if x.Level != OK && x.Fix == "" {
				t.Errorf("%s: %q has no fix", tc.name, x.What)
			}
			if strings.Contains(x.What+x.Fix, "secret-of-") {
				t.Errorf("%s: a finding shows a secret: %+v", tc.name, x)
			}
		}
	}
}

// setLogin changes the login with that ID in state.json.
func setLogin(t *testing.T, a *App, id string, fn func(*state.Login)) {
	t.Helper()
	err := a.Store.Update(func(st *state.State) error { fn(st.Logins[id]); return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoctorNoLogins(t *testing.T) {
	f := &fake{status: map[string]claude.AuthInfo{}, linked: true}
	a := newApp(t, map[string]*state.Login{}, "", f)
	a.Env = []string{"PATH=/bin", "HOME=" + t.TempDir(), "CLAUDE_CONFIG_DIR=" + t.TempDir()}

	fs := a.Doctor()
	if !has(fs, Warn, "no logins yet") || Problems(fs) != 0 {
		t.Errorf("findings = %+v; want the no-logins warning and no problem", fs)
	}
}

func TestDoctorStray(t *testing.T) {
	a, _ := healthy(t)
	cfg := t.TempDir()
	a.Env = []string{"PATH=/bin", "CLAUDE_CONFIG_DIR=" + cfg}
	if err := os.WriteFile(filepath.Join(cfg, ".credentials.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if fs := a.Doctor(); !has(fs, Warn, "wrote") {
		t.Errorf("findings = %+v; want the stray sign-in warning", fs)
	}
}
