package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/oauth"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

func TestAddNew(t *testing.T) {
	ctx := context.Background()

	t.Run("a token, the first login: the default", func(t *testing.T) {
		f := &fake{status: map[string]claude.AuthInfo{}}
		a := addApp(t, map[string]*state.Login{}, "", f)
		p := &prompts{secrets: []string{goodCode}}

		res, err := a.Add(ctx, "ci", state.Token, nil, p)
		if err != nil {
			t.Fatal(err)
		}
		if !res.BecameDefault || res.ExpiresAt.Sub(now) != oauth.Life || res.Account != "tok@example.com" {
			t.Errorf("result = %+v", res)
		}

		st := load(t, a)
		id, l, err := st.ByName("ci")
		if err != nil {
			t.Fatal(err)
		}
		if l.Kind != state.Token || l.Account != "tok@example.com" || l.TokenExpiresAt == nil ||
			strings.Join(l.Scopes, " ") != "user:inference user:profile" {
			t.Errorf("login = %+v", l)
		}

		fi, err := os.Stat(filepath.Join(a.Store.Dir(id), creds.TokenFile))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("token file: %v, %v; want 0600", fi, err)
		}
		b, _ := os.ReadFile(filepath.Join(a.Store.Dir(id), creds.TokenFile))
		if string(b) != goodToken+"\n" {
			t.Error("the token file does not hold the token")
		}
	})

	t.Run("an API key", func(t *testing.T) {
		f := &fake{status: map[string]claude.AuthInfo{}}
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
		f := &fake{status: map[string]claude.AuthInfo{}}
		a := addApp(t, map[string]*state.Login{}, "", f)

		res, err := a.Add(ctx, "home", state.SignIn, []string{"--email", "new@example.com"}, &prompts{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Account != "new@example.com" {
			t.Errorf("account %q", res.Account)
		}
		if _, l, _ := load(t, a).ByName("home"); l.Plan != "max" {
			t.Errorf("login = %+v", l)
		}
	})
}

// A rejected key or code, or a bad paste, saves nothing and leaves no
// directory behind.
func TestAddRejected(t *testing.T) {
	cases := []struct {
		name   string
		kind   state.Kind
		secret string
		want   string
	}{
		{"rejected key", state.APIKey, "sk-ant-api03-bad-key", "rejected the key"},
		{"refused code", state.Token, "bad-code-123#any-state", "invalid_grant"},
		{"not a key", state.APIKey, "hello", "not a sk-ant-"},
		{"a token for a code", state.Token, goodToken, "not the code"},
	}

	for _, tc := range cases {
		f := &fake{status: map[string]claude.AuthInfo{}}
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
	f := &fake{status: map[string]claude.AuthInfo{}}
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

// A token login shows the sign-in link and opens it, then reads the code.
// No code (Ctrl-C, empty stdin) saves nothing.
func TestAddTokenSignIn(t *testing.T) {
	f := &fake{status: map[string]claude.AuthInfo{}}
	a := addApp(t, map[string]*state.Login{}, "", f)
	p := &prompts{secrets: []string{goodCode}}

	if _, err := a.Add(context.Background(), "ci", state.Token, nil, p); err != nil {
		t.Fatal(err)
	}
	if len(p.opened) != 1 || !strings.Contains(p.opened[0], "scope=user%3Ainference%20user%3Aprofile") {
		t.Fatalf("opened %q; want the sign-in link", p.opened)
	}
	if !strings.Contains(strings.Join(p.notes, "\n"), p.opened[0]) {
		t.Errorf("notes %q; want the link", p.notes)
	}

	p = &prompts{}
	if _, err := a.Add(context.Background(), "ci2", state.Token, nil, p); err == nil {
		t.Error("no code: want an error")
	}
	if _, _, err := load(t, a).ByName("ci2"); err == nil {
		t.Error("no code: the login was saved")
	}
}

// A token login records the organization and the plan of its profile; when
// the profile cannot be read it is saved anyway, and says so.
func TestAddTokenProfile(t *testing.T) {
	f := &fake{status: map[string]claude.AuthInfo{}}
	a := addApp(t, map[string]*state.Login{}, "", f)

	if _, err := a.Add(context.Background(), "ci", state.Token, nil, &prompts{secrets: []string{goodCode}}); err != nil {
		t.Fatal(err)
	}
	if _, l, _ := load(t, a).ByName("ci"); l.Org != "Example Org" || l.Plan != "team" {
		t.Errorf("org %q, plan %q; want Example Org, team", l.Org, l.Plan)
	}

	a.Profile = func(context.Context, string) (anthropic.Profile, error) {
		return anthropic.Profile{}, errors.New("HTTP 500")
	}
	p := &prompts{secrets: []string{goodCode}}
	if _, err := a.Add(context.Background(), "ci2", state.Token, nil, p); err != nil {
		t.Fatal(err)
	}
	if _, l, _ := load(t, a).ByName("ci2"); l.Org != "" || l.Plan != "" {
		t.Errorf("org %q, plan %q; want none", l.Org, l.Plan)
	}
	if !strings.Contains(strings.Join(p.notes, "\n"), "Could not read the organization") {
		t.Errorf("notes %q; want the profile warning", p.notes)
	}
}

// ImportToken (ADR 0020) saves a token the user has: the profile is its
// check. A good profile fills the login in; 401 saves nothing; any other
// answer saves it as it is, with a note.
func TestImportToken(t *testing.T) {
	ctx := context.Background()

	t.Run("good: org, plan and scopes", func(t *testing.T) {
		a := addApp(t, map[string]*state.Login{}, "", &fake{status: map[string]claude.AuthInfo{}})

		res, err := a.ImportToken(ctx, "old", &prompts{secrets: []string{goodToken}})
		if err != nil {
			t.Fatal(err)
		}
		if !res.BecameDefault || !res.ExpiresAt.IsZero() || res.Account != "tok@example.com" {
			t.Errorf("result = %+v", res)
		}

		id, l, err := load(t, a).ByName("old")
		if err != nil {
			t.Fatal(err)
		}
		if l.Kind != state.Token || l.Org != "Example Org" || l.Plan != "team" || l.TokenExpiresAt != nil ||
			strings.Join(l.Scopes, " ") != "user:inference user:profile" {
			t.Errorf("login = %+v", l)
		}

		b, _ := os.ReadFile(filepath.Join(a.Store.Dir(id), creds.TokenFile))
		if string(b) != goodToken+"\n" {
			t.Error("the token file does not hold the token")
		}
	})

	cases := []struct {
		name    string
		secret  string
		profile error
		saved   bool
		note    string // in a note
		errText string // in the error
	}{
		{name: "rejected", secret: "sk-ant-oat01-other-token", errText: "rejected the token"}, //nolint:gosec // G101: a fake
		{name: "no profile scope", secret: goodToken, profile: anthropic.StatusError{Code: 403}, saved: true, note: "HTTP 403"},
		{name: "no answer", secret: goodToken, profile: errors.New("no route"), saved: true, note: "no route"},
		{name: "not a token", secret: goodKey, errText: "not a sk-ant-oat"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := addApp(t, map[string]*state.Login{}, "", &fake{status: map[string]claude.AuthInfo{}})
			if tc.profile != nil {
				a.Profile = func(context.Context, string) (anthropic.Profile, error) { return anthropic.Profile{}, tc.profile }
			}
			p := &prompts{secrets: []string{tc.secret}}

			_, err := a.ImportToken(ctx, "old", p)
			if (err == nil) != tc.saved || (err != nil && !strings.Contains(err.Error(), tc.errText)) {
				t.Fatalf("err = %v; saved %v, want error with %q", err, tc.saved, tc.errText)
			}

			_, l, lookup := load(t, a).ByName("old")
			if (lookup == nil) != tc.saved {
				t.Fatalf("saved = %v, want %v", lookup == nil, tc.saved)
			}
			if tc.saved && (len(l.Scopes) != 0 || l.Org != "") {
				t.Errorf("login = %+v; want no scopes and no org", l)
			}
			if tc.note != "" && !strings.Contains(strings.Join(p.notes, "\n"), tc.note) {
				t.Errorf("notes %q; want %q", p.notes, tc.note)
			}
			if err != nil && strings.Contains(err.Error(), tc.secret) {
				t.Error("the error holds the token")
			}
		})
	}

	t.Run("name taken", func(t *testing.T) {
		a := addApp(t, map[string]*state.Login{"a1": {Name: "work", Kind: state.SignIn}}, "a1", &fake{status: map[string]claude.AuthInfo{}})
		if _, err := a.ImportToken(ctx, "work", &prompts{secrets: []string{goodToken}}); err == nil {
			t.Error("want an error")
		}
	})
}

// A name that exists cannot take a new login of any kind, a token on top
// of a sign-in included.
func TestAddExisting(t *testing.T) {
	cases := []struct {
		name string
		kind state.Kind
	}{
		{"work", state.Token},
		{"work", state.SignIn},
		{"ci", state.Token},
		{"api", state.APIKey},
	}

	for _, tc := range cases {
		a, _ := three(t)

		_, err := a.Add(context.Background(), tc.name, tc.kind, nil, &prompts{secrets: []string{goodCode}})
		if err == nil || !strings.Contains(err.Error(), "cc-login renew "+tc.name) {
			t.Errorf("add %s %s = %v; want the renew hint", tc.name, tc.kind, err)
		}

		dirs, _ := os.ReadDir(filepath.Join(a.Store.Root, ".dirs"))
		if len(dirs) != 3 || len(load(t, a).Logins) != 3 {
			t.Errorf("add %s %s: %d directories, %d logins; want 3, 3", tc.name, tc.kind, len(dirs), len(load(t, a).Logins))
		}
	}
}
