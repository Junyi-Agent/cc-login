package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/oauth"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

func TestRenew(t *testing.T) {
	ctx := context.Background()
	logins := func() map[string]*state.Login {
		old := now.Add(-24 * time.Hour)
		return map[string]*state.Login{
			"a1": {Name: "work", Kind: state.SignIn},
			"t1": {Name: "ci", Kind: state.Token, Account: "ci@example.com", TokenExpiresAt: &old},
			"k1": {Name: "api", Kind: state.APIKey},
		}
	}

	t.Run("a sign-in: the new expiry and account, still the default", func(t *testing.T) {
		f := &fake{status: map[string]claude.AuthInfo{"a1": {SignedIn: true, ExpiresAt: now.Add(time.Hour)}}}
		a := addApp(t, logins(), "a1", f)

		res, err := a.Renew(ctx, "work", nil, &prompts{})
		if err != nil {
			t.Fatal(err)
		}
		if !res.ExpiresAt.Equal(now.Add(30*24*time.Hour)) || res.Account != "new@example.com" {
			t.Errorf("result = %+v", res)
		}
		if load(t, a).Default != "a1" {
			t.Error("work is no longer the default")
		}
	})

	t.Run("a failed sign-in changes nothing", func(t *testing.T) {
		f := &fake{status: map[string]claude.AuthInfo{"a1": {SignedIn: true}}}
		a := addApp(t, logins(), "a1", f)
		f.login = func([]string, []string) error { return claude.ErrStopped }

		if _, err := a.Renew(ctx, "work", nil, &prompts{}); !errors.Is(err, claude.ErrStopped) {
			t.Errorf("renew = %v", err)
		}
		if _, l, _ := load(t, a).ByName("work"); l.Account != "" {
			t.Errorf("login = %+v; want it unchanged", l)
		}
	})

	t.Run("a token: a new one, a new expiry, the token's account and scopes", func(t *testing.T) {
		f := &fake{status: map[string]claude.AuthInfo{}}
		a := addApp(t, logins(), "a1", f)

		res, err := a.Renew(ctx, "ci", nil, &prompts{secrets: []string{goodCode}})
		if err != nil {
			t.Fatal(err)
		}
		if res.ExpiresAt.Sub(now) != oauth.Life || res.Account != "tok@example.com" {
			t.Errorf("result = %+v", res)
		}
		if _, l, _ := load(t, a).ByName("ci"); len(l.Scopes) != 2 {
			t.Errorf("login = %+v; want the scopes", l)
		}
	})

	t.Run("a refused code changes nothing", func(t *testing.T) {
		f := &fake{status: map[string]claude.AuthInfo{}}
		a := addApp(t, logins(), "a1", f)

		if _, err := a.Renew(ctx, "ci", nil, &prompts{secrets: []string{"bad-code-123#s"}}); err == nil {
			t.Fatal("want an error")
		}
		if _, l, _ := load(t, a).ByName("ci"); l.Account != "ci@example.com" || l.Scopes != nil {
			t.Errorf("login = %+v; want it unchanged", l)
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
