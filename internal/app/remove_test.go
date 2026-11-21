package app

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

func TestRemove(t *testing.T) {
	ctx := context.Background()
	remove := func(a *App, name string) error {
		p, err := a.PlanRemove(ctx, name)
		if err != nil {
			return err
		}
		return a.Remove(ctx, p)
	}

	t.Run("the plan changes nothing", func(t *testing.T) {
		a, _ := three(t)

		if _, err := a.PlanRemove(ctx, "work"); err != nil {
			t.Fatal(err)
		}
		if len(load(t, a).Logins) != 3 {
			t.Error("a login is gone")
		}
	})

	t.Run("the default sign-in", func(t *testing.T) {
		a, f := three(t)
		var loggedOut []string
		f.logout = func(env []string) error {
			loggedOut = env
			return nil
		}

		plan, err := a.PlanRemove(ctx, "work")
		if err != nil {
			t.Fatal(err)
		}
		if !plan.LogOut || !plan.Default || plan.Kind != state.SignIn || plan.ID != "a1" {
			t.Errorf("plan = %+v", plan)
		}

		if err := a.Remove(ctx, plan); err != nil {
			t.Fatal(err)
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
		a, f := three(t)
		f.logout = func([]string) error {
			t.Error("logout ran for an API key login")
			return nil
		}

		if err := remove(a, "api"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a token: the sign-in under it, without the token", func(t *testing.T) {
		a, f := three(t)
		f.logout = func(env []string) error {
			if lookup(env, "CLAUDE_CODE_OAUTH_TOKEN") != "" {
				t.Error("logout got the token")
			}
			return nil
		}

		if err := remove(a, "ci"); err != nil {
			t.Fatal(err)
		}
		if lookup(f.statusTo["t1"], "CLAUDE_CODE_OAUTH_TOKEN") != "" {
			t.Error("the status check before the logout got the token")
		}
	})

	t.Run("a failed logout deletes nothing", func(t *testing.T) {
		a, f := three(t)
		f.logout = func([]string) error { return errors.New("offline") }

		err := remove(a, "work")
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
