package app

import (
	"context"
	"fmt"
	"os"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// RemovePlan is what removing a login does, for the confirmation.
type RemovePlan struct {
	ID      string
	Name    string
	Kind    state.Kind
	Default bool // the default login: afterwards there is none
	LogOut  bool // a sign-in is stored: claude auth logout revokes it first
}

// PlanRemove says what removing login name does.
func (a *App) PlanRemove(ctx context.Context, name string) (RemovePlan, error) {
	st, err := a.Store.Load()
	if err != nil {
		return RemovePlan{}, err
	}

	id, l, err := st.ByName(name)
	if err != nil {
		return RemovePlan{}, err
	}

	plan := RemovePlan{ID: id, Name: name, Kind: l.Kind, Default: st.Default == id}
	if l.Kind != state.APIKey {
		cs, err := a.Claude.AuthStatus(ctx, a.storedEnv(id))
		if err != nil {
			return RemovePlan{}, err
		}
		plan.LogOut = cs.SignedIn
	}

	return plan, nil
}

// Remove signs the login of plan p out and deletes it. The logout revokes a
// stored sign-in at Anthropic; a key or token stays valid until it is
// revoked or expires.
func (a *App) Remove(ctx context.Context, p RemovePlan) error {
	a.Log.Debug("remove", "login", p.Name, "id", p.ID, "logout", p.LogOut)
	if p.LogOut {
		if err := a.Claude.AuthLogout(ctx, a.storedEnv(p.ID)); err != nil {
			return fmt.Errorf("%w: claude auth logout failed: %w", ErrCancelled, err)
		}
	}

	err := a.Store.Update(func(st *state.State) error {
		delete(st.Logins, p.ID)
		if st.Default == p.ID {
			st.Default = ""
		}
		return nil
	})
	if err != nil {
		return err
	}

	return os.RemoveAll(a.Store.Dir(p.ID))
}
