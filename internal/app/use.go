package app

import (
	"context"
	"fmt"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Use makes login name the default. A login that is not signed in cannot be
// the default: claude would start signed out.
func (a *App) Use(ctx context.Context, name string) error {
	st, err := a.Store.Load()
	if err != nil {
		return err
	}

	id, l, err := st.ByName(name)
	if err != nil {
		return err
	}

	env, err := a.env(id, l)
	if err != nil {
		return err
	}

	cs, err := a.Claude.AuthStatus(ctx, env)
	if err != nil {
		return err
	}
	if !cs.SignedIn {
		return fmt.Errorf("login %s is signed out; sign it in with: cc-login renew %s", name, name)
	}

	return a.Store.Update(func(st *state.State) error {
		id, _, err := st.ByName(name)
		st.Default = id
		return err
	})
}
