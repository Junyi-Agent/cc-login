package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

var now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

// fake is the outside world of a test: claude's answers by credentials
// directory, access token expiries by directory, quotas by access token.
// It is the App's Claude.
type fake struct {
	mu       sync.Mutex
	status   map[string]claude.AuthInfo
	tokens   map[string]time.Time
	quotas   map[string]anthropic.Quota
	statusTo map[string][]string // the env each AuthStatus got, by directory

	pathErr error                          // Path fails with it
	login   func(env, args []string) error // claude auth login; nil does nothing
	logout  func(env []string) error       // claude auth logout; nil does nothing
}

func (f *fake) Path() (string, error) {
	if f.pathErr != nil {
		return "", f.pathErr
	}
	return "claude", nil
}

// AuthStatus answers from f.status, by the directory in env.
func (f *fake) AuthStatus(_ context.Context, env []string) (claude.AuthInfo, error) {
	dir := filepath.Base(lookup(env, "CLAUDE_SECURESTORAGE_CONFIG_DIR"))
	f.mu.Lock()
	defer f.mu.Unlock()

	f.statusTo[dir] = env
	return f.status[dir], nil
}

func (f *fake) AuthLogin(_ context.Context, env, args []string) error {
	if f.login == nil {
		return nil
	}
	return f.login(env, args)
}

func (f *fake) AuthLogout(_ context.Context, env []string) error {
	if f.logout == nil {
		return nil
	}
	return f.logout(env)
}

// newApp makes an App over a temporary root with these logins (by ID).
func newApp(t *testing.T, logins map[string]*state.Login, def string, f *fake) *App {
	t.Helper()
	s := &state.Store{Root: t.TempDir()}

	for id, l := range logins {
		dir := s.Dir(id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}

		file := map[state.Kind]string{state.APIKey: creds.APIKeyFile, state.Token: creds.TokenFile}[l.Kind]
		if file != "" {
			if err := os.WriteFile(filepath.Join(dir, file), []byte("secret-of-"+id+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	err := s.Update(func(st *state.State) error {
		st.Logins, st.Default = logins, def
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	f.statusTo = map[string][]string{}
	return &App{
		Store:  s,
		Env:    []string{"PATH=/bin", "ANTHROPIC_API_KEY=from-the-shell"},
		Now:    func() time.Time { return now },
		Claude: f,
		AccessToken: func(_ context.Context, dir string) (string, time.Time) {
			exp, ok := f.tokens[filepath.Base(dir)]
			if !ok {
				return "", time.Time{}
			}
			return "access-" + filepath.Base(dir), exp
		},
		Usage: func(_ context.Context, tok string) (anthropic.Quota, error) {
			q, ok := f.quotas[tok]
			if !ok {
				return anthropic.Quota{}, errors.New("HTTP 500")
			}
			return q, nil
		},
		Parallel: 2,
		Timeout:  time.Second,
		Log:      slog.New(slog.DiscardHandler),
	}
}

func used(f float64) *anthropic.Window { return &anthropic.Window{Used: &f} }

// three logins: work (sign-in, signed in, the default), ci (token), api (key).
func three(t *testing.T) (*App, *fake) {
	t.Helper()
	f := &fake{status: map[string]claude.AuthInfo{"a1": {SignedIn: true}, "t1": {SignedIn: true}}}
	a := newApp(t, map[string]*state.Login{
		"a1": {Name: "work", Kind: state.SignIn},
		"t1": {Name: "ci", Kind: state.Token},
		"k1": {Name: "api", Kind: state.APIKey},
	}, "a1", f)
	return a, f
}

func load(t *testing.T, a *App) *state.State {
	t.Helper()
	st, err := a.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// prompts is a Prompter that answers from lists and keeps the notes.
type prompts struct {
	secrets, lines []string
	notes          []string
}

func (p *prompts) Secret(_, _ string, _ func(string) error) (string, error) {
	if len(p.secrets) == 0 {
		return "", errors.New("no input")
	}
	s := p.secrets[0]
	p.secrets = p.secrets[1:]
	return s, nil
}

func (p *prompts) Line(_, _ string) (string, error) {
	if len(p.lines) == 0 {
		return "", nil
	}
	s := p.lines[0]
	p.lines = p.lines[1:]
	return s, nil
}

func (p *prompts) Note(msg string) { p.notes = append(p.notes, msg) }

// Fake values that the fake API accepts.
const (
	goodKey   = "sk-ant-api03-good-wxyz"  //nolint:gosec // G101: a fake
	goodToken = "sk-ant-oat01-good-token" //nolint:gosec // G101: a fake
)

// addApp is newApp with the API and claude auth login faked: the API
// accepts goodKey and goodToken (org o1), rejects others; a sign-in signs
// the directory in as new@example.com, expiring in signInDays.
func addApp(t *testing.T, logins map[string]*state.Login, def string, f *fake) *App {
	t.Helper()
	a := newApp(t, logins, def, f)

	a.CheckKey = func(_ context.Context, key string) (anthropic.Verdict, int) {
		if key == goodKey {
			return anthropic.Accepted, 200
		}
		return anthropic.Rejected, 401
	}
	a.CheckToken = func(_ context.Context, tok string) (anthropic.Verdict, string, int) {
		if tok == goodToken {
			return anthropic.Accepted, "o1", 200
		}
		return anthropic.Rejected, "", 401
	}
	f.login = func(env, _ []string) error {
		id := filepath.Base(lookup(env, "CLAUDE_SECURESTORAGE_CONFIG_DIR"))
		f.mu.Lock()
		defer f.mu.Unlock()
		f.status[id] = claude.AuthInfo{SignedIn: true, Email: "new@example.com", Plan: "max", OrgID: "o2", ExpiresAt: now.Add(30 * 24 * time.Hour)}
		return nil
	}

	return a
}
