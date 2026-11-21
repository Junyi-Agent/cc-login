package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Renew signs login name in again, also while it is signed in, so that it
// expires later; for a token login it saves a new OAuth token. The credentials
// directory stays the same, so the login stays the default if it was.
func (a *App) Renew(ctx context.Context, name string, args []string, p Prompter) (Saved, error) {
	st, err := a.Store.Load()
	if err != nil {
		return Saved{}, err
	}

	id, old, err := st.ByName(name)
	if err != nil {
		return Saved{}, err
	}

	switch {
	case old.Kind == state.APIKey:
		return Saved{}, fmt.Errorf("an API key does not expire; to change it, remove login %s and add it again", name)
	case old.Kind == state.Token && len(args) > 0:
		return Saved{}, errors.New("a token login takes no options for claude: renew signs it in again in the browser")
	}

	l := *old
	if err = a.authenticate(ctx, id, &l, args, p); err != nil {
		return Saved{}, err
	}

	err = a.Store.Update(func(st *state.State) error {
		st.Logins[id] = &l
		return nil
	})
	if err != nil {
		return Saved{}, err
	}

	return Saved{Name: name, Account: l.Account, ExpiresAt: a.expiry(ctx, id, &l)}, nil
}

// expiry is when login l's sign-in or token expires; zero when unknown.
func (a *App) expiry(ctx context.Context, id string, l *state.Login) time.Time {
	if l.Kind == state.Token {
		if l.TokenExpiresAt != nil {
			return *l.TokenExpiresAt
		}
		return time.Time{}
	}

	cs, err := a.Claude.AuthStatus(ctx, a.storedEnv(id))
	if err != nil || !cs.SignedIn {
		return time.Time{}
	}
	return cs.ExpiresAt
}
