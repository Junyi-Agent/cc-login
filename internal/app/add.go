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
		return a.saveKey(ctx, dir, l, p)
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

	setAccount(l, cs)
	return nil
}

// setAccount records in l the account claude auth status reported.
func setAccount(l *state.Login, cs claude.AuthInfo) {
	l.Account, l.OrgID, l.OrgName, l.Plan = cs.Email, cs.OrgID, cs.OrgName, cs.Plan
}

// saveKey reads an API key, checks it with the API and saves it.
func (a *App) saveKey(ctx context.Context, dir string, l *state.Login, p Prompter) error {
	valid := validPrefix("sk-ant-")
	key, err := p.Secret("API key for "+l.Name, "An Anthropic API key, sk-ant-...; it shows as *.", valid)
	if err != nil {
		return err
	}
	if err := valid(key); err != nil {
		return err
	}

	v, status := a.CheckKey(ctx, key)
	a.Log.Debug("check key", "login", l.Name, "verdict", v, "status", status)
	switch v {
	case anthropic.Rejected:
		return fmt.Errorf("the Anthropic API rejected the key (HTTP %d); nothing saved", status)
	case anthropic.Unknown:
		p.Note(fmt.Sprintf("Could not check the key (%s); saving it unchecked.", httpStatus(status)))
	case anthropic.Accepted:
	}

	if err := creds.WriteSecret(dir, state.APIKey, key); err != nil {
		return err
	}

	l.KeyHint = key[len(key)-4:]
	return nil
}

// saveToken reads a token from claude setup-token, checks it with the API
// and saves it. A token cannot read its account, so it asks for one to show
// when the login has none.
func (a *App) saveToken(ctx context.Context, dir string, l *state.Login, p Prompter) error {
	p.Note("In another terminal run: claude setup-token. A browser opens; choose the account of login " + l.Name + ", then paste the token it prints (sk-ant-oat...).")
	valid := validPrefix("sk-ant-oat")
	tok, err := p.Secret("Token for "+l.Name, "From claude setup-token; it shows as *.", valid)
	if err != nil {
		return err
	}
	if err := valid(tok); err != nil {
		return err
	}

	v, org, status := a.CheckToken(ctx, tok)
	a.Log.Debug("check token", "login", l.Name, "verdict", v, "status", status, "org", org)
	switch v {
	case anthropic.Rejected:
		return fmt.Errorf("the Anthropic API rejected the token (HTTP %d); nothing saved", status)
	case anthropic.Unknown:
		p.Note(fmt.Sprintf("Could not check the token (%s); saving it unchecked.", httpStatus(status)))
	case anthropic.Accepted:
	}

	if err := creds.WriteSecret(dir, state.Token, tok); err != nil {
		return err
	}

	if l.Account == "" {
		if acct, err := p.Line("Account of "+l.Name, "The email to show in cc-login list; a token cannot read it. Enter skips."); err == nil && acct != "" {
			l.Account = strings.Join(strings.Fields(acct), "")
		}
	}

	a.tokenOrg(l, org)

	now := a.Now().UTC()
	expires := now.Add(tokenLife)
	l.TokenSavedAt, l.TokenExpiresAt = &now, &expires
	return nil
}

// tokenOrg records the token's organization. The organization's name, and
// the plan of the same account in it, come from other logins: a token
// cannot read them.
func (a *App) tokenOrg(l *state.Login, org string) {
	if org == "" {
		return
	}

	if l.OrgID != org {
		l.OrgName, l.Plan = "", ""
	}
	l.OrgID = org

	st, err := a.Store.Load()
	if err != nil {
		return
	}
	for _, other := range st.Logins {
		if other.Name == l.Name || other.OrgID != org {
			continue
		}
		if l.OrgName == "" {
			l.OrgName = other.OrgName
		}
		if l.Plan == "" && l.Account != "" && other.Account == l.Account {
			l.Plan = other.Plan
		}
	}
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
