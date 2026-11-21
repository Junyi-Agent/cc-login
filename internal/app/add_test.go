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
	"github.com/Junyi-Agent/cc-login/internal/state"
)

func TestAddNew(t *testing.T) {
	ctx := context.Background()

	t.Run("a token, the first login: the default", func(t *testing.T) {
		f := &fake{status: map[string]claude.AuthInfo{}}
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

// At a terminal, add runs claude setup-token in the new login's directory,
// with no credentials of the shell, then reads the pasted token. A failed
// claude still lets a token be pasted; Ctrl-C saves nothing. Without a
// terminal claude setup-token does not run.
func TestAddTokenRunsSetupToken(t *testing.T) {
	cases := []struct {
		name     string
		terminal bool
		setupErr error
		wantRun  bool
		wantErr  string
		wantNote string
	}{
		{"at a terminal", true, nil, true, "", "Running claude setup-token"},
		{"claude fails", true, errors.New("exit status 1"), true, "", "run it in another terminal"},
		{"Ctrl-C", true, claude.ErrStopped, true, "claude setup-token: stopped", ""},
		{"no terminal", false, nil, false, "", "run in a terminal"},
	}

	for _, tc := range cases {
		var ran bool
		var env []string
		f := &fake{status: map[string]claude.AuthInfo{}, setup: func(e []string) error {
			ran, env = true, e
			return tc.setupErr
		}}
		a := addApp(t, map[string]*state.Login{}, "", f)
		p := &prompts{secrets: []string{goodToken}, terminal: tc.terminal}

		_, err := a.Add(context.Background(), "ci", state.Token, nil, p)

		switch {
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: %v; want %q", tc.name, err, tc.wantErr)
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		}
		if ran != tc.wantRun {
			t.Errorf("%s: setup-token ran %t; want %t", tc.name, ran, tc.wantRun)
		}
		if ran && (lookup(env, "ANTHROPIC_API_KEY") != "" || lookup(env, "CLAUDE_SECURESTORAGE_CONFIG_DIR") == "") {
			t.Errorf("%s: setup-token env %q; want the login's directory and no key", tc.name, env)
		}
		if tc.wantNote != "" && !strings.Contains(strings.Join(p.notes, "\n"), tc.wantNote) {
			t.Errorf("%s: notes %q; want %q", tc.name, p.notes, tc.wantNote)
		}
		if logins := len(load(t, a).Logins); (tc.wantErr == "") != (logins == 1) {
			t.Errorf("%s: %d logins saved", tc.name, logins)
		}
	}
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

		_, err := a.Add(context.Background(), tc.name, tc.kind, nil, &prompts{secrets: []string{goodToken}})
		if err == nil || !strings.Contains(err.Error(), "cc-login renew "+tc.name) {
			t.Errorf("add %s %s = %v; want the renew hint", tc.name, tc.kind, err)
		}

		dirs, _ := os.ReadDir(filepath.Join(a.Store.Root, ".dirs"))
		if len(dirs) != 3 || len(load(t, a).Logins) != 3 {
			t.Errorf("add %s %s: %d directories, %d logins; want 3, 3", tc.name, tc.kind, len(dirs), len(load(t, a).Logins))
		}
	}
}
