package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

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
		f := &fake{status: map[string]claude.AuthInfo{"a1": {SignedIn: true, ExpiresAt: now.Add(time.Hour)}}}
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
		f := &fake{status: map[string]claude.AuthInfo{"a1": {SignedIn: true, ExpiresAt: now.Add(60 * 24 * time.Hour)}}}
		a := addApp(t, logins(), "a1", f)

		if _, err := a.Renew(ctx, "work", nil, &prompts{}); err == nil || !strings.Contains(err.Error(), "did not move") {
			t.Errorf("renew = %v", err)
		}
	})

	t.Run("a failed sign-in keeps the old one", func(t *testing.T) {
		f := &fake{status: map[string]claude.AuthInfo{"a1": {SignedIn: true}}}
		a := addApp(t, logins(), "a1", f)
		f.login = func([]string, []string) error { return claude.ErrStopped }

		_, err := a.Renew(ctx, "work", nil, &prompts{})
		if !errors.Is(err, claude.ErrStopped) || !strings.Contains(err.Error(), "keeps the sign-in it had") {
			t.Errorf("renew = %v", err)
		}
	})

	t.Run("a token: a new one, a new expiry, the same account", func(t *testing.T) {
		f := &fake{status: map[string]claude.AuthInfo{}}
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
		a := addApp(t, logins(), "a1", &fake{status: map[string]claude.AuthInfo{}})
		if _, err := a.Renew(ctx, tc.name, tc.args, &prompts{}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("renew %s = %v; want %q", tc.name, err, tc.want)
		}
	}
}
