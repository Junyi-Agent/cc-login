package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// three logins: work (sign-in, signed in, the default), ci (token), api (key).
func three(t *testing.T) (*App, *fake) {
	t.Helper()
	f := &fake{status: map[string]claude.Status{"a1": {SignedIn: true}, "t1": {SignedIn: true}}}
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

func TestUse(t *testing.T) {
	a, _ := three(t)
	ctx := context.Background()

	if err := a.Use(ctx, "ci"); err != nil {
		t.Fatal(err)
	}
	if st := load(t, a); st.Default != "t1" {
		t.Errorf("default = %q, want t1", st.Default)
	}

	// api is signed out in the fake: it cannot be the default.
	if err := a.Use(ctx, "api"); err == nil || !strings.Contains(err.Error(), "signed out") {
		t.Errorf("use api = %v; want signed out", err)
	}
	if err := a.Use(ctx, "nope"); !errors.Is(err, state.ErrNoLogin) {
		t.Errorf("use nope = %v; want ErrNoLogin", err)
	}
	if st := load(t, a); st.Default != "t1" {
		t.Errorf("a failed use changed the default to %q", st.Default)
	}
}

func TestRename(t *testing.T) {
	a, _ := three(t)

	if err := a.Rename("work", "home"); err != nil {
		t.Fatal(err)
	}
	st := load(t, a)
	if st.Logins["a1"].Name != "home" || st.Default != "a1" {
		t.Errorf("after rename: %+v, default %q", st.Logins["a1"], st.Default)
	}

	cases := []struct{ old, name string }{
		{"home", "ci"},    // taken
		{"home", "-x"},    // not a name
		{"nope", "other"}, // no such login
	}
	for _, tc := range cases {
		if err := a.Rename(tc.old, tc.name); err == nil {
			t.Errorf("rename %s %s: want an error", tc.old, tc.name)
		}
	}
}

func TestRemove(t *testing.T) {
	ctx := context.Background()
	yes := func(RemovePlan) bool { return true }

	t.Run("no", func(t *testing.T) {
		a, _ := three(t)
		err := a.Remove(ctx, "work", func(RemovePlan) bool { return false })
		if !errors.Is(err, ErrCancelled) || len(load(t, a).Logins) != 3 {
			t.Errorf("remove, answered no = %v", err)
		}
	})

	t.Run("the default sign-in", func(t *testing.T) {
		a, _ := three(t)
		var plan RemovePlan
		var loggedOut []string
		a.Logout = func(_ context.Context, _ string, env []string) error {
			loggedOut = env
			return nil
		}

		err := a.Remove(ctx, "work", func(p RemovePlan) bool { plan = p; return true })
		if err != nil {
			t.Fatal(err)
		}

		if !plan.LogOut || !plan.Default || plan.Kind != state.SignIn {
			t.Errorf("plan = %+v", plan)
		}
		if lookup(loggedOut, "CLAUDE_SECURESTORAGE_CONFIG_DIR") != a.Store.Dir("a1") {
			t.Errorf("logout ran in %q", lookup(loggedOut, "CLAUDE_SECURESTORAGE_CONFIG_DIR"))
		}

		st := load(t, a)
		if st.Logins["a1"] != nil || st.Default != "" {
			t.Errorf("after remove: logins %v, default %q", st.Names(), st.Default)
		}
		if _, err := os.Stat(a.Store.Dir("a1")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the credentials directory is still there: %v", err)
		}
	})

	t.Run("an API key: nothing to log out", func(t *testing.T) {
		a, _ := three(t)
		a.Logout = func(context.Context, string, []string) error {
			t.Error("logout ran for an API key login")
			return nil
		}

		if err := a.Remove(ctx, "api", yes); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a token: the sign-in under it, without the token", func(t *testing.T) {
		a, f := three(t)
		a.Logout = func(_ context.Context, _ string, env []string) error {
			if lookup(env, "CLAUDE_CODE_OAUTH_TOKEN") != "" {
				t.Error("logout got the token")
			}
			return nil
		}

		if err := a.Remove(ctx, "ci", yes); err != nil {
			t.Fatal(err)
		}
		if lookup(f.statusTo["t1"], "CLAUDE_CODE_OAUTH_TOKEN") != "" {
			t.Error("the status check before the logout got the token")
		}
	})

	t.Run("a failed logout deletes nothing", func(t *testing.T) {
		a, _ := three(t)
		a.Logout = func(context.Context, string, []string) error { return errors.New("offline") }

		err := a.Remove(ctx, "work", yes)
		if !errors.Is(err, ErrCancelled) {
			t.Errorf("remove with a failed logout = %v", err)
		}
		if load(t, a).Logins["a1"] == nil {
			t.Error("the login is gone")
		}
		if _, err := os.Stat(a.Store.Dir("a1")); err != nil {
			t.Errorf("the credentials directory is gone: %v", err)
		}
	})
}

func TestLaunch(t *testing.T) {
	cases := []struct {
		name    string
		env     []string
		want    string // the login, "" for an error
		key     string // ANTHROPIC_API_KEY in claude's environment
		wantErr string
	}{
		{name: "", want: "work"},
		{name: "api", want: "api", key: "secret-of-k1"},
		{name: "", env: []string{"CC_LOGIN_ID=t1"}, want: "ci"},
		{name: "", env: []string{"CC_LOGIN_ID=gone"}, want: "work"},
		{name: "work", env: []string{"CC_LOGIN_ID=t1"}, want: "work"},
		{name: "nope", wantErr: "no such login"},
		{name: "", env: []string{"CC_LOGIN_DEPTH=10"}, wantErr: "started itself"},
	}

	for _, tc := range cases {
		a, _ := three(t)
		a.Env = append(a.Env, tc.env...)

		l, err := a.Launch(tc.name)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Launch(%q) %v = %v; want %q", tc.name, tc.env, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Launch(%q) %v: %v", tc.name, tc.env, err)
		}

		if l.Name != tc.want || lookup(l.Env, "ANTHROPIC_API_KEY") != tc.key {
			t.Errorf("Launch(%q) %v = %s with key %q; want %s, %q", tc.name, tc.env, l.Name, lookup(l.Env, "ANTHROPIC_API_KEY"), tc.want, tc.key)
		}
		if got := lookup(l.Env, "CC_LOGIN_DEPTH"); got == "" || filepath.Base(lookup(l.Env, "CLAUDE_SECURESTORAGE_CONFIG_DIR")) != lookup(l.Env, "CC_LOGIN_ID") {
			t.Errorf("Launch(%q): depth %q, id %q", tc.name, got, lookup(l.Env, "CC_LOGIN_ID"))
		}
	}

	a, _ := three(t)
	if err := a.Store.Update(func(st *state.State) error { st.Default = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Launch(""); !errors.Is(err, ErrNoDefault) {
		t.Errorf("Launch with no default = %v", err)
	}
}
