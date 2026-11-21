package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Renew signs login name in again, also while it is signed in, so that it
// expires later; for a token login it saves a new token. The credentials
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
		return Saved{}, errors.New("a token login takes no options for claude: renew saves a new token from claude setup-token")
	}

	before := a.expiry(ctx, id, old)

	l := *old
	if err = a.authenticate(ctx, id, &l, args, p); err != nil {
		return Saved{}, a.renewFailed(ctx, id, old, err)
	}

	if old.OrgID != "" && l.OrgID != "" && l.OrgID != old.OrgID {
		p.Note(fmt.Sprintf("Login %s was in organization %s and is now in %s: the browser may have picked another one.",
			name, cmp.Or(old.OrgName, old.OrgID), cmp.Or(l.OrgName, l.OrgID)))
	}

	err = a.Store.Update(func(st *state.State) error {
		st.Logins[id] = &l
		return nil
	})
	if err != nil {
		return Saved{}, err
	}

	res := Saved{Name: name, Account: l.Account, ExpiresAt: a.expiry(ctx, id, &l)}
	if l.Kind == state.SignIn && !res.ExpiresAt.IsZero() && !res.ExpiresAt.After(before) {
		return res, errors.New("signed in again, but the expiry did not move")
	}
	return res, nil
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

// renewFailed says whether the login kept the sign-in it had.
func (a *App) renewFailed(ctx context.Context, id string, l *state.Login, err error) error {
	if l.Kind == state.Token {
		return err
	}

	// After Ctrl-C too.
	if cs, errStatus := a.Claude.AuthStatus(context.WithoutCancel(ctx), a.storedEnv(id)); errStatus == nil && cs.SignedIn {
		return fmt.Errorf("%w; login %s keeps the sign-in it had", err, l.Name)
	}
	return fmt.Errorf("%w; login %s is signed out", err, l.Name)
}
