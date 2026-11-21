package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/quota"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

var now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

// fake is the outside world of a test: claude's answers by credentials
// directory, access tokens by directory, quotas by access token.
type fake struct {
	mu       sync.Mutex
	status   map[string]claude.Status
	tokens   map[string]creds.Result
	quotas   map[string]quota.Quota
	statusTo map[string][]string // the env each AuthStatus got, by directory
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

		file := map[state.Kind]string{state.APIKey: state.APIKeyFile, state.Token: state.TokenFile}[l.Kind]
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
		Store:      s,
		Env:        []string{"PATH=/bin", "ANTHROPIC_API_KEY=from-the-shell"},
		Now:        func() time.Time { return now },
		FindClaude: func() (string, error) { return "claude", nil },
		AuthStatus: func(_ context.Context, _ string, env []string) (claude.Status, error) {
			dir := lookup(env, "CLAUDE_SECURESTORAGE_CONFIG_DIR")
			f.mu.Lock()
			defer f.mu.Unlock()
			f.statusTo[filepath.Base(dir)] = env
			return f.status[filepath.Base(dir)], nil
		},
		AccessToken: func(_ context.Context, dir string, _ time.Time) (string, creds.Result) {
			res, ok := f.tokens[filepath.Base(dir)]
			if !ok {
				return "", creds.None
			}
			return "access-" + filepath.Base(dir), res
		},
		Usage: func(_ context.Context, tok string) (quota.Quota, error) {
			q, ok := f.quotas[tok]
			if !ok {
				return quota.Quota{}, errors.New("HTTP 500")
			}
			return q, nil
		},
		QuotaCache: filepath.Join(t.TempDir(), "cache.json"),
		Parallel:   2,
		Timeout:    time.Second,
	}
}

func pct(n int) *int { return &n }

func TestList(t *testing.T) {
	inDays := func(d int) *time.Time { t := now.Add(time.Duration(d) * 24 * time.Hour); return &t }
	logins := map[string]*state.Login{
		"a1": {Name: "work", Kind: state.SignIn, Account: "me@example.com", Plan: "team"},
		"b2": {Name: "soon", Kind: state.SignIn},
		"c3": {Name: "gone", Kind: state.SignIn},
		"d4": {Name: "old", Kind: state.SignIn},
		"e5": {Name: "ci", Kind: state.Token, TokenExpiresAt: inDays(300)},
		"f6": {Name: "api", Kind: state.APIKey, KeyHint: "wxyz"},
		"g7": {Name: "stale", Kind: state.SignIn},
		"h8": {Name: "flaky", Kind: state.SignIn},
	}
	f := &fake{
		status: map[string]claude.Status{
			"a1": {SignedIn: true, ExpiresAt: *inDays(30)},
			"b2": {SignedIn: true, ExpiresAt: *inDays(2)},
			"d4": {ExpiresAt: *inDays(-1)},
			"e5": {SignedIn: true},
			"f6": {SignedIn: true},
			"g7": {SignedIn: true},
			"h8": {SignedIn: true},
		},
		tokens: map[string]creds.Result{"a1": creds.OK, "b2": creds.OK, "g7": creds.Expired, "h8": creds.OK},
		quotas: map[string]quota.Quota{
			"access-a1": {FiveHour: quota.Window{Used: pct(30)}},
			"access-b2": {SevenDay: quota.Window{Used: pct(90)}},
		},
	}
	a := newApp(t, logins, "a1", f)

	// The token login has a cached quota.
	if err := os.WriteFile(a.QuotaCache, []byte(`{"dir:e5": {"five_hour": {"used_percentage": 40}, "at": 1795000000}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	views, err := a.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]View{}
	var names []string
	for _, v := range views {
		byName[v.Name] = v
		names = append(names, v.Name)
	}

	if !slices.IsSorted(names) || len(names) != len(logins) {
		t.Errorf("names = %q, want all, sorted", names)
	}

	cases := []struct {
		name   string
		status Status
		source QuotaSource
		gap    QuotaGap
	}{
		{"work", SignedIn, QuotaLive, GapNone},
		{"soon", Expiring, QuotaLive, GapNone},
		{"gone", SignedOut, QuotaNone, GapNone},
		{"old", Expired, QuotaNone, GapNone},
		{"ci", SignedIn, QuotaCache, GapToken},
		{"api", SignedIn, QuotaNone, GapNoLogin},
		{"stale", SignedIn, QuotaNone, GapStale},
		{"flaky", SignedIn, QuotaNone, GapError},
	}

	for _, tc := range cases {
		v := byName[tc.name]
		if v.Status != tc.status || v.QuotaSource != tc.source || v.QuotaGap != tc.gap {
			t.Errorf("%s: status %v, source %v, gap %v; want %v, %v, %v",
				tc.name, v.Status, v.QuotaSource, v.QuotaGap, tc.status, tc.source, tc.gap)
		}
	}

	if !byName["work"].Default || byName["soon"].Default {
		t.Error("only work is the default")
	}
	if got := byName["ci"].ExpiresAt; !got.Equal(*inDays(300)) {
		t.Errorf("ci expires %v; want the expiry state.json keeps", got)
	}
	if got := byName["api"]; got.Account != "API key …wxyz" || got.Plan != "api" {
		t.Errorf("api: account %q, plan %q", got.Account, got.Plan)
	}
	if got := byName["ci"].CachedAt; !got.Equal(time.Unix(1795000000, 0)) {
		t.Errorf("ci cached at %v", got)
	}
}

// Each claude gets its own login's key or token, never the shell's or
// another login's.
func TestListKeepsSecretsApart(t *testing.T) {
	logins := map[string]*state.Login{
		"k1": {Name: "key", Kind: state.APIKey},
		"t1": {Name: "tok", Kind: state.Token},
		"s1": {Name: "sign", Kind: state.SignIn},
	}
	f := &fake{status: map[string]claude.Status{}}
	a := newApp(t, logins, "", f)

	if _, err := a.List(context.Background()); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		id, key, token string
	}{
		{"k1", "secret-of-k1", ""},
		{"t1", "", "secret-of-t1"},
		{"s1", "", ""},
	}

	for _, tc := range cases {
		env := f.statusTo[tc.id]
		if got := lookup(env, "ANTHROPIC_API_KEY"); got != tc.key {
			t.Errorf("%s: ANTHROPIC_API_KEY = %q, want %q", tc.id, got, tc.key)
		}
		if got := lookup(env, "CLAUDE_CODE_OAUTH_TOKEN"); got != tc.token {
			t.Errorf("%s: CLAUDE_CODE_OAUTH_TOKEN = %q, want %q", tc.id, got, tc.token)
		}
		if got := lookup(env, "CLAUDE_SECURESTORAGE_CONFIG_DIR"); got != a.Store.Dir(tc.id) {
			t.Errorf("%s: the directory is %q", tc.id, got)
		}
	}
}

func TestListNoLogins(t *testing.T) {
	f := &fake{}
	a := newApp(t, map[string]*state.Login{}, "", f)
	a.FindClaude = func() (string, error) { return "", claude.ErrNotFound }

	views, err := a.List(context.Background())
	if err != nil || len(views) != 0 {
		t.Errorf("List = %v, %v; want nothing, and no need for claude", views, err)
	}
}

func TestListCancelled(t *testing.T) {
	f := &fake{status: map[string]claude.Status{}}
	a := newApp(t, map[string]*state.Login{"a1": {Name: "work", Kind: state.SignIn}}, "", f)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := a.List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("List after Ctrl-C = %v; want context.Canceled", err)
	}
}
