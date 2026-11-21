package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// tokenLife is how long a setup-token token lasts; claude does not say.
const tokenLife = 365 * 24 * time.Hour

// Saved is what Add, Adopt or Renew saved, for the message.
type Saved struct {
	Name          string
	Account       string
	BecameDefault bool      // Add and Adopt: there was no default
	ExpiresAt     time.Time // zero when unknown
}

// Add saves a new login name of that kind: a token from claude setup-token,
// an API key, or a sign-in with claude auth login (args are its options).
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
		key, err := a.saveSecret(ctx, dir, l, p)
		if err != nil {
			return err
		}
		l.KeyHint = key[len(key)-4:]
		return nil

	case state.Token:
		if err := a.setupToken(ctx, dir, l.Name, p); err != nil {
			return err
		}
		if _, err := a.saveSecret(ctx, dir, l, p); err != nil {
			return err
		}
		askAccount(l, p)
		expires := a.Now().UTC().Add(tokenLife)
		l.TokenExpiresAt = &expires
		return nil

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

// saveSecret reads the API key or the token of login l, checks it with the
// API and saves it in dir. It returns the secret.
func (a *App) saveSecret(ctx context.Context, dir string, l *state.Login, p Prompter) (string, error) {
	noun, title, help, prefix, check := "key", "API key", "An Anthropic API key, sk-ant-...; it shows as *.", "sk-ant-", a.CheckKey
	if l.Kind == state.Token {
		noun, title, help, prefix, check = "token", "Token", "The token claude setup-token printed (sk-ant-oat...); it shows as *.", "sk-ant-oat", a.CheckToken
	}
	valid := validPrefix(prefix)

	s, err := p.Secret(title+" for "+l.Name, help, valid)
	if err != nil {
		return "", err
	}
	if err := valid(s); err != nil {
		return "", err
	}

	v, status := check(ctx, s)
	a.Log.Debug("check "+noun, "login", l.Name, "verdict", v, "status", status)
	switch v {
	case anthropic.Rejected:
		return "", fmt.Errorf("the Anthropic API rejected the %s (HTTP %d); nothing saved", noun, status)
	case anthropic.Unknown:
		p.Note(fmt.Sprintf("Could not check the %s (%s); saving it unchecked.", noun, httpStatus(status)))
	case anthropic.Accepted:
	}

	return s, creds.WriteSecret(dir, l.Kind, s)
}

// askAccount asks for the email of token login l, to show in list: a token
// cannot read it. A login that has one keeps it.
func askAccount(l *state.Login, p Prompter) {
	if l.Account != "" {
		return
	}

	acct, err := p.Line("Account of "+l.Name, "The email to show in cc-login list; a token cannot read it. Enter skips.")
	if err == nil && acct != "" {
		l.Account = strings.Join(strings.Fields(acct), "")
	}
}

// setupToken runs claude setup-token on the user's terminal for login name,
// so the token to paste is on the screen. Without a terminal it says where
// the token comes from: claude setup-token needs one. When claude fails, a
// token from a run elsewhere can still be pasted.
func (a *App) setupToken(ctx context.Context, dir, name string, p Prompter) error {
	if !p.Terminal() {
		p.Note("The token comes from claude setup-token, run in a terminal: choose the account of login " + name + " in the browser it opens.")
		return nil
	}

	p.Note("Running claude setup-token: a browser opens; choose the account of login " + name + ", then paste the token it prints.")
	a.Log.Debug("claude setup-token", "login", name)
	err := a.Claude.SetupToken(ctx, creds.StoredEnv(a.Env, dir))

	switch {
	case errors.Is(err, claude.ErrStopped):
		return fmt.Errorf("claude setup-token: %w", err)
	case err != nil:
		p.Note(fmt.Sprintf("claude setup-token failed (%v); run it in another terminal and paste the token it prints.", err))
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
