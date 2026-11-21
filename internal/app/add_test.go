package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

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
	a.SignIn = func(_ context.Context, _ string, env, _ []string) error {
		id := filepath.Base(lookup(env, "CLAUDE_SECURESTORAGE_CONFIG_DIR"))
		f.mu.Lock()
		defer f.mu.Unlock()
		f.status[id] = claude.Status{SignedIn: true, Email: "new@example.com", Plan: "max", OrgID: "o2", ExpiresAt: now.Add(30 * 24 * time.Hour)}
		return nil
	}

	return a
}

func TestAddNew(t *testing.T) {
	ctx := context.Background()

	t.Run("a token, the first login: the default", func(t *testing.T) {
		f := &fake{status: map[string]claude.Status{}}
		a := addApp(t, map[string]*state.Login{"x1": {Name: "team", Kind: state.SignIn, OrgID: "o1", OrgName: "Example", Account: "me@example.com", Plan: "team"}}, "", f)
		p := &prompts{secrets: []string{goodToken}, lines: []string{"me@example.com"}}

		res, err := a.Add(ctx, "ci", state.Token, nil, p)
		if err != nil {
			t.Fatal(err)
		}
		if !res.BecameDefault || res.ExpiresAt.Sub(now) != tokenLife {
			t.Errorf("result = %+v", res)
		}

		st := load(t, a)
		id, l, err := st.ByName("ci")
		if err != nil {
			t.Fatal(err)
		}
		if l.Kind != state.Token || l.OrgID != "o1" || l.OrgName != "Example" || l.Plan != "team" || l.Account != "me@example.com" {
			t.Errorf("login = %+v; want the org name and plan of the other login in o1", l)
		}

		fi, err := os.Stat(filepath.Join(a.Store.Dir(id), state.TokenFile))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("token file: %v, %v; want 0600", fi, err)
		}
		b, _ := os.ReadFile(filepath.Join(a.Store.Dir(id), state.TokenFile))
		if string(b) != goodToken+"\n" {
			t.Error("the token file does not hold the token")
		}
	})

	t.Run("an API key", func(t *testing.T) {
		f := &fake{status: map[string]claude.Status{}}
		a := addApp(t, map[string]*state.Login{"a1": {Name: "work", Kind: state.SignIn}}, "a1", f)

		res, err := a.Add(ctx, "api", state.APIKey, nil, &prompts{secrets: []string{goodKey}})
		if err != nil {
			t.Fatal(err)
		}
		if res.BecameDefault {
			t.Error("there is a default already")
		}
		if _, l, _ := load(t, a).ByName("api"); l.KeyHint != "wxyz" {
			t.Errorf("key hint %q", l.KeyHint)
		}
	})

	t.Run("a sign-in", func(t *testing.T) {
		f := &fake{status: map[string]claude.Status{}}
		a := addApp(t, map[string]*state.Login{}, "", f)

		res, err := a.Add(ctx, "home", state.SignIn, []string{"--email", "new@example.com"}, &prompts{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Account != "new@example.com" {
			t.Errorf("account %q", res.Account)
		}
		if _, l, _ := load(t, a).ByName("home"); l.Plan != "max" || l.OrgID != "o2" {
			t.Errorf("login = %+v", l)
		}
	})
}

// A rejected key or token, or a bad paste, saves nothing and leaves no
// directory behind.
func TestAddRejected(t *testing.T) {
	cases := []struct {
		name   string
		kind   state.Kind
		secret string
		want   string
	}{
		{"rejected key", state.APIKey, "sk-ant-api03-bad-key", "rejected the key"},
		{"rejected token", state.Token, "sk-ant-oat01-bad-token", "rejected the token"},
		{"not a key", state.APIKey, "hello", "not a sk-ant-"},
		{"a key for a token", state.Token, goodKey, "not a sk-ant-oat"},
	}

	for _, tc := range cases {
		f := &fake{status: map[string]claude.Status{}}
		a := addApp(t, map[string]*state.Login{}, "", f)

		_, err := a.Add(context.Background(), "x", tc.kind, nil, &prompts{secrets: []string{tc.secret}})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v; want %q", tc.name, err, tc.want)
		}
		if strings.Contains(err.Error(), tc.secret) && len(tc.secret) > 8 {
			t.Errorf("%s: the error shows the secret", tc.name)
		}

		dirs, _ := os.ReadDir(filepath.Join(a.Store.Root, ".dirs"))
		if len(dirs) != 0 || len(load(t, a).Logins) != 0 {
			t.Errorf("%s: left %d directories behind", tc.name, len(dirs))
		}
	}
}

// An unchecked key (no answer from the API) is saved, with a note.
func TestAddUnchecked(t *testing.T) {
	f := &fake{status: map[string]claude.Status{}}
	a := addApp(t, map[string]*state.Login{}, "", f)
	a.CheckKey = func(context.Context, string) (anthropic.Verdict, int) { return anthropic.Unknown, 0 }
	p := &prompts{secrets: []string{goodKey}}

	if _, err := a.Add(context.Background(), "api", state.APIKey, nil, p); err != nil {
		t.Fatal(err)
	}
	if len(p.notes) != 1 || !strings.Contains(p.notes[0], "no response") {
		t.Errorf("notes = %q", p.notes)
	}
}

func TestAddExisting(t *testing.T) {
	logins := func() map[string]*state.Login {
		return map[string]*state.Login{
			"a1": {Name: "work", Kind: state.SignIn, Account: "me@example.com"},
			"t1": {Name: "ci", Kind: state.Token},
			"k1": {Name: "api", Kind: state.APIKey},
			"o1": {Name: "off", Kind: state.SignIn},
		}
	}
	signed := map[string]claude.Status{"a1": {SignedIn: true}, "t1": {SignedIn: true}}

	cases := []struct {
		name string
		kind state.Kind
		want string // "" for success
	}{
		{"ci", state.Token, "cc-login renew ci"},
		{"api", state.Token, "an API key login"},
		{"work", state.SignIn, "is signed in"},
		{"work", state.APIKey, "is signed in"},
		{"work", state.Token, ""}, // on top of the sign-in
		{"off", state.SignIn, ""}, // signed out: its directory is reused
	}

	for _, tc := range cases {
		f := &fake{status: map[string]claude.Status{}}
		for k, v := range signed {
			f.status[k] = v
		}
		a := addApp(t, logins(), "a1", f)
		p := &prompts{secrets: []string{goodToken}}

		res, err := a.Add(context.Background(), tc.name, tc.kind, nil, p)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("add %s %s: %v", tc.name, tc.kind, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("add %s %s = %v; want %q", tc.name, tc.kind, err, tc.want)
		case tc.want == "":
			st := load(t, a)
			id, l, _ := st.ByName(tc.name)
			if len(st.Logins) != 4 || (tc.name == "work" && (id != "a1" || l.Kind != state.Token || !res.OverSignIn)) {
				t.Errorf("add %s %s: logins %v, %s is %+v", tc.name, tc.kind, st.Names(), id, l)
			}
		}
	}
}

func TestRenew(t *testing.T) {
	ctx := context.Background()
	logins := func() map[string]*state.Login {
		old := now.Add(-24 * time.Hour)
		return map[string]*state.Login{
			"a1": {Name: "work", Kind: state.SignIn, OrgID: "o1", OrgName: "Old"},
			"t1": {Name: "ci", Kind: state.Token, Account: "ci@example.com", TokenExpiresAt: &old},
			"k1": {Name: "api", Kind: state.APIKey},
		}
	}

	t.Run("a sign-in: the expiry moves, the org change is noted", func(t *testing.T) {
		f := &fake{status: map[string]claude.Status{"a1": {SignedIn: true, ExpiresAt: now.Add(time.Hour)}}}
		a := addApp(t, logins(), "a1", f)
		p := &prompts{}

		res, err := a.Renew(ctx, "work", nil, p)
		if err != nil {
			t.Fatal(err)
		}
		if !res.ExpiresAt.Equal(now.Add(30 * 24 * time.Hour)) {
			t.Errorf("expires %v", res.ExpiresAt)
		}
		if len(p.notes) != 1 || !strings.Contains(p.notes[0], "another one") {
			t.Errorf("notes = %q", p.notes)
		}
		if load(t, a).Default != "a1" {
			t.Error("work is no longer the default")
		}
	})

	t.Run("a sign-in whose expiry did not move", func(t *testing.T) {
		f := &fake{status: map[string]claude.Status{"a1": {SignedIn: true, ExpiresAt: now.Add(60 * 24 * time.Hour)}}}
		a := addApp(t, logins(), "a1", f)

		if _, err := a.Renew(ctx, "work", nil, &prompts{}); err == nil || !strings.Contains(err.Error(), "did not move") {
			t.Errorf("renew = %v", err)
		}
	})

	t.Run("a failed sign-in keeps the old one", func(t *testing.T) {
		f := &fake{status: map[string]claude.Status{"a1": {SignedIn: true}}}
		a := addApp(t, logins(), "a1", f)
		a.SignIn = func(context.Context, string, []string, []string) error { return claude.ErrStopped }

		_, err := a.Renew(ctx, "work", nil, &prompts{})
		if !errors.Is(err, claude.ErrStopped) || !strings.Contains(err.Error(), "keeps the sign-in it had") {
			t.Errorf("renew = %v", err)
		}
	})

	t.Run("a token: a new one, a new expiry, the same account", func(t *testing.T) {
		f := &fake{status: map[string]claude.Status{}}
		a := addApp(t, logins(), "a1", f)

		res, err := a.Renew(ctx, "ci", nil, &prompts{secrets: []string{goodToken}})
		if err != nil {
			t.Fatal(err)
		}
		if res.ExpiresAt.Sub(now) != tokenLife || res.Account != "ci@example.com" {
			t.Errorf("result = %+v", res)
		}
	})

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"api", nil, "does not expire"},
		{"ci", []string{"--email", "x"}, "takes no options"},
		{"nope", nil, "no such login"},
	}
	for _, tc := range cases {
		a := addApp(t, logins(), "a1", &fake{status: map[string]claude.Status{}})
		if _, err := a.Renew(ctx, tc.name, tc.args, &prompts{}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("renew %s = %v; want %q", tc.name, err, tc.want)
		}
	}
}

func TestAdopt(t *testing.T) {
	f := &fake{status: map[string]claude.Status{}}
	a := addApp(t, map[string]*state.Login{"a1": {Name: "work", Kind: state.SignIn}}, "", f)

	cfg := t.TempDir()
	a.Env = append(a.Env, "CLAUDE_CONFIG_DIR="+cfg)
	src := filepath.Join(cfg, ".credentials.json")

	if _, err := a.Adopt(context.Background(), "stray"); err == nil {
		t.Error("adopt with no file: want an error")
	}

	if err := os.WriteFile(src, []byte(`{"claudeAiOauth":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Adopt(context.Background(), "work"); err == nil {
		t.Error("adopt into an existing name: want an error")
	}

	res, err := a.Adopt(context.Background(), "stray")
	if err != nil {
		t.Fatal(err)
	}
	if !res.BecameDefault {
		t.Error("there was no default; stray should be it")
	}

	id, _, _ := load(t, a).ByName("stray")
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Error("the file is still in ~/.claude: it must move, not be copied")
	}
	if _, err := os.Stat(filepath.Join(a.Store.Dir(id), ".credentials.json")); err != nil {
		t.Errorf("the file is not in the login: %v", err)
	}
}
