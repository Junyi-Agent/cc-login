package listing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/quota"
	"github.com/Junyi-Agent/cc-login/internal/store"
)

func init() { time.Local = time.UTC }

func TestLeft(t *testing.T) {
	cases := map[int64]string{1: "1m", 60: "1m", 61: "2m", 3599: "60m", 3600: "1h", 2*86400 - 1: "47h", 2 * 86400: "2d", 30 * 86400: "30d"}
	for s, want := range cases {
		if got := Left(s); got != want {
			t.Errorf("Left(%d) = %q, want %q", s, got, want)
		}
	}
}

func TestRecordFields(t *testing.T) {
	cases := []struct {
		rec         map[string]any
		email, plan string
	}{
		{map[string]any{"email": "a@x", "subscriptionType": "max"}, "a@x", "max"},
		{map[string]any{"authMethod": "api_key", "apiKeyHint": "…wxyz"}, "API key …wxyz", "api"},
		{map[string]any{"authMethod": "api_key"}, "API key ?", "api"},
		{map[string]any{"email": nil}, "-", "-"},
	}

	for _, c := range cases {
		e, p := recordFields(c.rec)
		if e != c.email || p != c.plan {
			t.Errorf("recordFields(%v) = %q %q", c.rec, e, p)
		}
	}
}

func ip(n int) *int { return &n }

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func TestRender(t *testing.T) {
	n := now.Unix()
	logins := []Login{
		{Name: "console", Email: "API key …wxyz", Plan: "api", SignedIn: true},
		{Name: "personal", Email: "me@example.com", Plan: "max", SignedIn: true, Expiry: n + 28*86400, QuotaWhy: "expired"},
		{Name: "work", Email: "me@company.com", Plan: "team", SignedIn: true, Expiry: n + 4*3600, QuotaOK: true,
			Quota: quota.Quota{FiveHour: quota.Window{Used: ip(27), Reset: n + 3600}, SevenDay: quota.Window{Used: ip(76), Reset: n + 2*86400}}},
		{Name: "tok", Email: "t@x", Plan: "-", SignedIn: true, Token: true, Expiry: n + 364*86400, QuotaOK: true, Cached: true, CachedAt: n - 600,
			Quota: quota.Quota{FiveHour: quota.Window{Used: ip(100)}}},
		{Name: "old", Email: "o@x", Plan: "pro", Expiry: n - 3*86400},
		{Name: "out", Email: "-", Plan: "-"},
	}

	var b strings.Builder
	if err := Render(&b, logins, "work", now, Style{}); err != nil {
		t.Fatal(err)
	}

	want := `  NAME      ACCOUNT         PLAN  5H LEFT    WEEK LEFT    STATUS      EXPIRES
  console   API key …wxyz   api   -          -          ● signed in   never
  personal  me@example.com  max   ?          ?          ● signed in   in 28d   12-18 12:00
▸ work      me@company.com  team  73% 13:00  24% 11-22  ● signed in   in 4h    11-20 16:00
  tok       t@x             -     0%*        -          ● token       in 364d  2027-11-19 12:00
  old       o@x             pro   -          -          ● expired     3d ago   11-17 12:00
  out       -               -     -          -          ○ signed out  -

  Sign in again with: cc-login renew NAME
  Quota unknown for personal: the access token expired; a claude started with the login refreshes it: cc-login run NAME
  * the quota as a claude last saw it (quota cache): tok 11:50
`
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestRenderNarrow(t *testing.T) {
	logins := []Login{{Name: "a", Email: "someone.with.a.long.address@example.com", Plan: "max", SignedIn: true, Expiry: now.Unix() + 86400*30}}

	var b strings.Builder
	if err := Render(&b, logins, "", now, Style{Color: true, Cols: 70}); err != nil {
		t.Fatal(err)
	}

	out := b.String()
	if strings.Contains(out, "12-20 12:00") {
		t.Error("too wide: the date and time should go first")
	}
	if !strings.Contains(out, "…") {
		t.Error("too wide: the account should be shortened")
	}
	if !strings.Contains(out, "No default login") {
		t.Error("no default: want the note")
	}
}

// fakeStore makes a store with logins sign (a sign-in), key (an API key) and
// tok (a token login), each with a record.
func fakeStore(t *testing.T) *store.Store {
	root := t.TempDir()

	for name, file := range map[string]string{"sign": "", "key": "api-key", "tok": "oauth-token"} {
		dir := filepath.Join(root, ".dirs", strings.ToUpper(name))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(".dirs/"+strings.ToUpper(name), filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
		if file != "" {
			if err := os.WriteFile(filepath.Join(dir, file), []byte("secret\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	for name, rec := range map[string]string{
		"sign": `{"email":"s@x","subscriptionType":"max"}`,
		"key":  `{"authMethod":"api_key","apiKeyHint":"…cret"}`,
		"tok":  `{"email":"t@x","loginExpiresAt":"2027-11-20T12:00:00Z"}`,
	} {
		if err := os.WriteFile(filepath.Join(root, name+".json"), []byte(rec), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return &store.Store{Root: root}
}

func TestProber(t *testing.T) {
	s := fakeStore(t)

	var mu sync.Mutex // AuthStatus runs in one goroutine per login
	envs := map[string][]string{}
	p := &Prober{
		Store: s, Now: now, CachePath: filepath.Join(t.TempDir(), "none.json"),
		AuthStatus: func(_ context.Context, env []string) ([]byte, bool) {
			for _, kv := range env {
				if dir, ok := strings.CutPrefix(kv, "CLAUDE_SECURESTORAGE_CONFIG_DIR="); ok {
					mu.Lock()
					envs[filepath.Base(dir)] = env
					mu.Unlock()
				}
			}
			return []byte(`{"loggedIn":true,"loginExpiresAt":"2026-12-01T00:00:00.000Z"}`), true
		},
		AccessToken: func(_ context.Context, dir string, _ time.Time) (string, creds.Result) {
			if filepath.Base(dir) == "SIGN" {
				return "at", creds.OK
			}
			return "", creds.None
		},
		FetchQuota: func(_ context.Context, tok string) (quota.Quota, error) {
			if tok != "at" {
				return quota.Quota{}, errors.New("bad token")
			}
			return quota.Quota{FiveHour: quota.Window{Used: ip(10)}}, nil
		},
	}

	ls := p.All(context.Background(), s.Names())
	got := map[string]Login{}
	for _, l := range ls {
		got[l.Name] = l
	}

	if l := got["sign"]; !l.QuotaOK || l.Expiry != time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC).Unix() || l.Email != "s@x" {
		t.Errorf("sign: %+v", l)
	}
	if l := got["key"]; l.QuotaOK || l.QuotaWhy != "" || l.Plan != "api" {
		t.Errorf("key: %+v", l)
	}
	// A token login's expiry comes from its record, not from claude.
	if l := got["tok"]; !l.Token || l.Expiry != time.Date(2027, 11, 20, 12, 0, 0, 0, time.UTC).Unix() || l.QuotaOK {
		t.Errorf("tok: %+v", l)
	}

	// Each login gets its own key or token and none of another's.
	has := func(env []string, k string) bool {
		for _, kv := range env {
			if strings.HasPrefix(kv, k+"=") {
				return true
			}
		}
		return false
	}

	if !has(envs["KEY"], "ANTHROPIC_API_KEY") || has(envs["KEY"], "CLAUDE_CODE_OAUTH_TOKEN") {
		t.Error("key: want ANTHROPIC_API_KEY only")
	}
	if !has(envs["TOK"], "CLAUDE_CODE_OAUTH_TOKEN") || has(envs["TOK"], "ANTHROPIC_API_KEY") {
		t.Error("tok: want CLAUDE_CODE_OAUTH_TOKEN only")
	}
	if has(envs["SIGN"], "ANTHROPIC_API_KEY") || has(envs["SIGN"], "CLAUDE_CODE_OAUTH_TOKEN") {
		t.Error("sign: want neither")
	}
}

func TestProberDropsInheritedKey(t *testing.T) {
	s := fakeStore(t)

	var seen []string
	p := &Prober{
		Store: s, Now: now, Env: []string{"ANTHROPIC_API_KEY=default-key", "PATH=/bin"},
		AuthStatus:  func(_ context.Context, env []string) ([]byte, bool) { seen = env; return nil, false },
		AccessToken: func(context.Context, string, time.Time) (string, creds.Result) { return "", creds.None },
	}
	p.One(context.Background(), "sign")

	for _, kv := range seen {
		if strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") {
			t.Fatal("the default login's key leaked into another login")
		}
	}
}
