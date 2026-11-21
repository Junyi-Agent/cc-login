package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/oauth"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Saved is what Add, Adopt or Renew saved, for the message.
type Saved struct {
	Name          string
	Account       string
	BecameDefault bool      // Add and Adopt: there was no default
	ExpiresAt     time.Time // zero when unknown
}

// Add saves a new login name of that kind: an OAuth token (ADR 0017), an
// API key, or a sign-in with claude auth login (args are its options).
// It becomes the default when there is none.
func (a *App) Add(ctx context.Context, name string, kind state.Kind, args []string, p Prompter) (Saved, error) {
	if err := a.checkNewName(name); err != nil {
		return Saved{}, err
	}

	id, err := a.Store.NewDir()
	if err != nil {
		return Saved{}, err
	}

	l := &state.Login{Name: name, Kind: kind, CreatedAt: a.Now().UTC()}
	if err := a.authenticate(ctx, id, l, args, p); err != nil {
		_ = os.RemoveAll(a.Store.Dir(id)) // nothing left behind
		return Saved{}, err
	}

	return a.addLogin(id, l)
}

// checkNewName checks that name can name a new login.
func (a *App) checkNewName(name string) error {
	if err := state.CheckName(name); err != nil {
		return err
	}

	st, err := a.Store.Load()
	if err != nil {
		return err
	}

	if _, _, err := st.ByName(name); err == nil {
		return fmt.Errorf("login %s exists; sign it in again with cc-login renew %s, or remove it first", name, name)
	}
	return nil
}

// addLogin puts new login l, in the directory of id, into state.json. It
// becomes the default when there is none.
func (a *App) addLogin(id string, l *state.Login) (Saved, error) {
	res := Saved{Name: l.Name, Account: l.Account}
	if l.TokenExpiresAt != nil {
		res.ExpiresAt = *l.TokenExpiresAt
	}

	err := a.Store.Update(func(st *state.State) error {
		st.Logins[id] = l
		if st.Logins[st.Default] == nil {
			st.Default, res.BecameDefault = id, true
		}
		return nil
	})
	return res, err
}

// authenticate puts the credentials of login l into the directory of id, and
// records in l what it learns of the account. Add and Renew use it.
func (a *App) authenticate(ctx context.Context, id string, l *state.Login, args []string, p Prompter) error {
	dir := a.Store.Dir(id)

	switch l.Kind {
	case state.APIKey:
		key, err := a.saveKey(ctx, dir, l.Name, p)
		if err != nil {
			return err
		}
		l.KeyHint = key[len(key)-4:]
		return nil

	case state.Token:
		return a.saveToken(ctx, dir, l, p)

	case state.SignIn:
	}

	return a.saveSignIn(ctx, id, l, args)
}

// saveSignIn runs claude auth login for login l, then reads the account.
// Claude Code stores the sign-in in the login's directory.
func (a *App) saveSignIn(ctx context.Context, id string, l *state.Login, args []string) error {
	env := a.storedEnv(id)

	a.Log.Debug("claude auth login", "login", l.Name, "id", id, "options", len(args))
	if err := a.Claude.AuthLogin(ctx, env, args); err != nil {
		return fmt.Errorf("claude auth login: %w", err)
	}

	cs, err := a.Claude.AuthStatus(ctx, env)
	if err != nil {
		return err
	}
	if !cs.SignedIn {
		return errors.New("claude auth login ended, but the login is not signed in")
	}

	l.Account, l.Plan = cs.Email, cs.Plan
	return nil
}

// saveKey reads the API key of login name, checks it with the API and
// saves it in dir. It returns the key.
func (a *App) saveKey(ctx context.Context, dir, name string, p Prompter) (string, error) {
	valid := validPrefix("sk-ant-")

	key, err := p.Secret("API key for "+name, "An Anthropic API key, sk-ant-...; it shows as *.", valid)
	if err != nil {
		return "", err
	}
	if err := valid(key); err != nil {
		return "", err
	}

	v, status := a.CheckKey(ctx, key)
	a.Log.Debug("check key", "login", name, "verdict", v, "status", status)
	switch v {
	case anthropic.Rejected:
		return "", fmt.Errorf("the Anthropic API rejected the key (HTTP %d); nothing saved", status)
	case anthropic.Unknown:
		p.Note(fmt.Sprintf("Could not check the key (%s); saving it unchecked.", httpStatus(status)))
	case anthropic.Accepted:
	}

	return key, creds.WriteSecret(dir, state.APIKey, key)
}

// saveToken signs token login l in with OAuth (ADR 0017): the user allows
// access in the browser and pastes the code it shows; cc-login trades the
// code for a token, saves the token in dir, and records in l the account,
// the scopes and when the token expires.
func (a *App) saveToken(ctx context.Context, dir string, l *state.Login, p Prompter) error {
	f := oauth.Start()
	p.Note("Open this link, choose the account of login " + l.Name + ", allow access, then paste the code it shows:")
	p.Note(f.URL)
	p.Open(f.URL)

	code, err := p.Secret("Code for "+l.Name, "The code the browser shows (CODE#STATE); it shows as *.", oauth.CheckCode)
	if err != nil {
		return err
	}

	tok, err := a.Exchange(ctx, f, code)
	a.Log.Debug("oauth token", "login", l.Name, "ok", err == nil)
	if err != nil {
		return err
	}

	if err := creds.WriteSecret(dir, state.Token, tok.AccessToken); err != nil {
		return err
	}

	expires := a.Now().UTC().Add(tok.Life())
	l.TokenExpiresAt, l.Scopes = &expires, tok.Scopes()
	if tok.Account.Email != "" {
		l.Account = tok.Account.Email
	}
	return nil
}

// validPrefix checks a pasted key or token: the prefix, and no spaces.
func validPrefix(prefix string) func(string) error {
	return func(s string) error {
		if !strings.HasPrefix(s, prefix) || len(s) <= len(prefix)+4 || strings.ContainsAny(s, " \t\r\n") {
			return fmt.Errorf("that is not a %s... value", prefix)
		}
		return nil
	}
}

func httpStatus(status int) string {
	if status == 0 {
		return "no response"
	}
	return fmt.Sprintf("HTTP %d", status)
}
