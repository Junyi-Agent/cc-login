package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Prompter asks the user. On a terminal Secret reads without echo and Line
// asks a question; without one, Secret reads a line of stdin and Line
// answers "".
type Prompter interface {
	Secret(title, help string, valid func(string) error) (string, error)
	Line(title, help string) (string, error)
	Note(msg string) // a line for the user, on stderr
}

// tokenLife is how long a setup-token token lasts; claude does not say.
const tokenLife = 365 * 24 * time.Hour

// Added is what Add or Renew did, for the message.
type Added struct {
	Name          string
	Kind          state.Kind
	Account       string
	BecameDefault bool
	OverSignIn    bool      // a token added to a login with a stored sign-in
	ExpiresAt     time.Time // zero when unknown
}

// Add saves login name of that kind: a token from claude setup-token, an
// API key, or a sign-in with claude auth login (args are its options). A
// token can also go on top of an existing sign-in login, where it overrides
// the stored sign-in. A new login becomes the default when there is none.
func (a *App) Add(ctx context.Context, name string, kind state.Kind, args []string, p Prompter) (Added, error) {
	if err := state.CheckName(name); err != nil {
		return Added{}, err
	}

	st, err := a.Store.Load()
	if err != nil {
		return Added{}, err
	}

	id, old, err := a.addTarget(ctx, st, name, kind)
	if err != nil {
		return Added{}, err
	}

	isNew := old == nil
	if isNew {
		if id, err = a.Store.NewDir(); err != nil {
			return Added{}, err
		}
	}

	l, err := a.authenticate(ctx, id, name, kind, args, old, p)
	if err != nil {
		if isNew {
			_ = os.RemoveAll(a.Store.Dir(id)) // nothing left behind
		}
		return Added{}, err
	}

	res := Added{Name: name, Kind: kind, Account: l.Account, OverSignIn: old != nil && old.Kind == state.SignIn && kind == state.Token}
	if l.TokenExpiresAt != nil {
		res.ExpiresAt = *l.TokenExpiresAt
	}

	err = a.Store.Update(func(st *state.State) error {
		if isNew {
			l.CreatedAt = a.Now().UTC()
		}
		st.Logins[id] = l
		if st.Logins[st.Default] == nil {
			st.Default, res.BecameDefault = id, true
		}
		return nil
	})
	return res, err
}

// addTarget checks that login name can take a new login of that kind. It
// returns the existing login and its ID, or nil for a new one.
func (a *App) addTarget(ctx context.Context, st *state.State, name string, kind state.Kind) (string, *state.Login, error) {
	id, old, err := st.ByName(name)
	if errors.Is(err, state.ErrNoLogin) {
		return "", nil, nil
	}

	switch {
	case kind == state.Token && old.Kind == state.APIKey:
		return "", nil, fmt.Errorf("login %s is an API key login; add the token as a separate login", name)
	case kind == state.Token && old.Kind == state.Token:
		return "", nil, fmt.Errorf("login %s has a token; save a new one with: cc-login renew %s", name, name)
	case kind == state.Token:
		return id, old, nil // on top of the sign-in
	}

	signed, err := a.signedIn(ctx, id, old, false)
	if err != nil {
		return "", nil, err
	}
	if signed {
		return "", nil, fmt.Errorf("login %s is signed in; sign it in again with: cc-login renew %s", name, name)
	}
	return id, old, nil // signed out: reuse its directory
}

// authenticate puts the credentials of the login into its directory and
// returns its new entry for state.json.
func (a *App) authenticate(ctx context.Context, id, name string, kind state.Kind, args []string, old *state.Login, p Prompter) (*state.Login, error) {
	dir := a.Store.Dir(id)
	l := &state.Login{Name: name, Kind: kind}
	if old != nil {
		*l = *old
		l.Kind = kind
	}

	switch kind {
	case state.APIKey:
		return l, a.saveKey(ctx, dir, l, p)
	case state.Token:
		return l, a.saveToken(ctx, dir, l, old == nil || old.Account == "", p)
	case state.SignIn:
	}

	path, err := a.FindClaude()
	if err != nil {
		return nil, err
	}
	env := claude.StoredEnv(a.Env, dir)

	if err = a.SignIn(ctx, path, env, args); err != nil {
		return nil, fmt.Errorf("claude auth login: %w", err)
	}

	cs, err := a.AuthStatus(ctx, path, env)
	if err != nil {
		return nil, err
	}
	if !cs.SignedIn {
		return nil, errors.New("claude auth login ended, but the login is not signed in")
	}

	l.Account, l.OrgID, l.OrgName, l.Plan = cs.Email, cs.OrgID, cs.OrgName, cs.Plan
	return l, nil
}

// saveKey reads an API key, checks it with the API and saves it.
func (a *App) saveKey(ctx context.Context, dir string, l *state.Login, p Prompter) error {
	valid := validPrefix("sk-ant-")
	key, err := p.Secret("API key for "+l.Name, "An Anthropic API key, sk-ant-...; it is not shown.", valid)
	if err != nil {
		return err
	}
	if err := valid(key); err != nil {
		return err
	}

	switch v, status := a.CheckKey(ctx, key); v {
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
	l.Account, l.OrgID, l.OrgName, l.Plan = "", "", "", ""
	return nil
}

// saveToken reads a token from claude setup-token, checks it with the API
// and saves it. askAccount says to ask for the account to show: a token
// cannot read it.
func (a *App) saveToken(ctx context.Context, dir string, l *state.Login, askAccount bool, p Prompter) error {
	p.Note("In another terminal run: claude setup-token. A browser opens; choose the account of login " + l.Name + ", then paste the token it prints (sk-ant-oat...).")
	valid := validPrefix("sk-ant-oat")
	tok, err := p.Secret("Token for "+l.Name, "From claude setup-token; it is not shown.", valid)
	if err != nil {
		return err
	}
	if err := valid(tok); err != nil {
		return err
	}

	v, org, status := a.CheckToken(ctx, tok)
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

	if askAccount {
		if acct, err := p.Line("Account of "+l.Name, "The email to show in cc-login list; a token cannot read it. Enter skips."); err == nil && acct != "" {
			l.Account = strings.Join(strings.Fields(acct), "")
		}
	}

	a.tokenOrg(l, org, p)

	now := a.Now().UTC()
	expires := now.Add(tokenLife)
	l.TokenSavedAt, l.TokenExpiresAt = &now, &expires
	return nil
}

// tokenOrg records the token's organization. The organization's name, and
// the plan of the same account in it, come from other logins: a token
// cannot read them.
func (a *App) tokenOrg(l *state.Login, org string, p Prompter) {
	if org == "" {
		return
	}

	if l.OrgID != "" && l.OrgID != org {
		p.Note("The token belongs to another organization than the login had; the login now shows the token's.")
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

// Renew signs login name in again, also while it is signed in, so that it
// expires later; for a token login it saves a new token. The credentials
// directory stays the same, so the login stays the default if it was.
func (a *App) Renew(ctx context.Context, name string, args []string, p Prompter) (Added, error) {
	st, err := a.Store.Load()
	if err != nil {
		return Added{}, err
	}

	id, old, err := st.ByName(name)
	if err != nil {
		return Added{}, err
	}

	switch {
	case old.Kind == state.APIKey:
		return Added{}, fmt.Errorf("an API key does not expire; to change it, remove login %s and add it again", name)
	case old.Kind == state.Token && len(args) > 0:
		return Added{}, errors.New("a token login takes no options for claude: renew saves a new token from claude setup-token")
	}

	before := a.expiry(ctx, id, old)

	l, err := a.authenticate(ctx, id, name, old.Kind, args, old, p)
	if err != nil {
		return Added{}, a.renewFailed(ctx, id, old, err)
	}

	if old.OrgID != "" && l.OrgID != "" && l.OrgID != old.OrgID {
		p.Note(fmt.Sprintf("Login %s was in organization %s and is now in %s: the browser may have picked another one.", name, orDash(old.OrgName), orDash(l.OrgName)))
	}

	err = a.Store.Update(func(st *state.State) error {
		st.Logins[id] = l
		return nil
	})
	if err != nil {
		return Added{}, err
	}

	res := Added{Name: name, Kind: l.Kind, Account: l.Account}
	res.ExpiresAt = a.expiry(ctx, id, l)
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

	path, err := a.FindClaude()
	if err != nil {
		return time.Time{}
	}
	cs, err := a.AuthStatus(ctx, path, claude.StoredEnv(a.Env, a.Store.Dir(id)))
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

	if ok, _ := a.signedIn(context.WithoutCancel(ctx), id, l, true); ok {
		return fmt.Errorf("%w; login %s keeps the sign-in it had", err, l.Name)
	}
	return fmt.Errorf("%w; login %s is signed out", err, l.Name)
}

// StrayCredentials is the sign-in a claude run without cc-login wrote to
// ~/.claude (or $CLAUDE_CONFIG_DIR); "" when there is none.
func (a *App) StrayCredentials() string {
	cfg := lookup(a.Env, "CLAUDE_CONFIG_DIR")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".claude")
	}

	path := filepath.Join(cfg, ".credentials.json")
	if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() { //nolint:gosec // G703: the user's own CLAUDE_CONFIG_DIR, read as Claude Code reads it
		return ""
	}
	return path
}

// Adopt moves the stray sign-in (StrayCredentials) into a new login name.
// It moves the file, never copies it: two copies of one sign-in invalidate
// each other at the first token refresh. claude sessions that read it from
// ~/.claude lose their credentials.
func (a *App) Adopt(ctx context.Context, name string) (Added, error) {
	src := a.StrayCredentials()
	if src == "" {
		return Added{}, errors.New("there is no ~/.claude/.credentials.json to adopt")
	}

	if err := state.CheckName(name); err != nil {
		return Added{}, err
	}

	st, err := a.Store.Load()
	if err != nil {
		return Added{}, err
	}
	if _, _, errName := st.ByName(name); errName == nil {
		return Added{}, fmt.Errorf("login %s exists", name)
	}

	id, err := a.Store.NewDir()
	if err != nil {
		return Added{}, err
	}

	dir := a.Store.Dir(id)
	if err = os.Rename(src, filepath.Join(dir, ".credentials.json")); err != nil {
		_ = os.RemoveAll(dir)
		return Added{}, fmt.Errorf("move %s: %w (it stays where it is)", src, err)
	}

	l := &state.Login{Name: name, Kind: state.SignIn, CreatedAt: a.Now().UTC()}
	if path, errFind := a.FindClaude(); errFind == nil {
		if cs, errStatus := a.AuthStatus(ctx, path, claude.StoredEnv(a.Env, dir)); errStatus == nil {
			l.Account, l.OrgID, l.OrgName, l.Plan = cs.Email, cs.OrgID, cs.OrgName, cs.Plan
		}
	}

	res := Added{Name: name, Kind: state.SignIn, Account: l.Account}
	err = a.Store.Update(func(st *state.State) error {
		st.Logins[id] = l
		if st.Logins[st.Default] == nil {
			st.Default, res.BecameDefault = id, true
		}
		return nil
	})
	return res, err
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

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
