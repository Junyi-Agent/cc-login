package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Errors of the commands.
var (
	ErrNoDefault = errors.New("no default login; choose one with: cc-login use NAME")
	ErrCancelled = errors.New("nothing changed")
)

// maxDepth is how many cc-logins may run inside each other: a claude can
// start a claude (claude -p, hooks), but a claude link that leads back to
// cc-login without a real claude between them loops.
const maxDepth = 10

// signedIn runs claude auth status for the login with that ID; stored says
// to look at the sign-in stored in its directory only, without its key or
// token.
func (a *App) signedIn(ctx context.Context, id string, l *state.Login, stored bool) (bool, error) {
	path, err := a.FindClaude()
	if err != nil {
		return false, err
	}

	dir := a.Store.Dir(id)
	env := claude.StoredEnv(a.Env, dir)
	if !stored {
		if env, err = claude.Env(a.Env, dir, l.Kind); err != nil {
			return false, err
		}
	}

	st, err := a.AuthStatus(ctx, path, env)
	return st.SignedIn, err
}

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

	ok, err := a.signedIn(ctx, id, l, false)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("login %s is signed out; sign it in with: cc-login renew %s", name, name)
	}

	return a.Store.Update(func(st *state.State) error {
		id, _, err := st.ByName(name)
		st.Default = id
		return err
	})
}

// Rename renames login old to name. The credentials directory keeps its
// path, so the login stays signed in, and running claudes keep working.
func (a *App) Rename(old, name string) error {
	if err := state.CheckName(name); err != nil {
		return err
	}

	return a.Store.Update(func(st *state.State) error {
		_, l, err := st.ByName(old)
		if err != nil {
			return err
		}

		if _, _, err := st.ByName(name); err == nil {
			return fmt.Errorf("login %s exists", name)
		}

		l.Name = name
		return nil
	})
}

// RemovePlan is what Remove is about to do, for the confirmation.
type RemovePlan struct {
	Name    string
	Kind    state.Kind
	Default bool // the default login: afterwards there is none
	LogOut  bool // a sign-in is stored: claude auth logout revokes it first
}

// Remove signs login name out and deletes it, after confirm agrees to the
// plan. The logout revokes a stored sign-in at Anthropic; a key or token
// stays valid until it is revoked or expires.
func (a *App) Remove(ctx context.Context, name string, confirm func(RemovePlan) bool) error {
	st, err := a.Store.Load()
	if err != nil {
		return err
	}

	id, l, err := st.ByName(name)
	if err != nil {
		return err
	}

	plan := RemovePlan{Name: name, Kind: l.Kind, Default: st.Default == id}
	if l.Kind != state.APIKey {
		if plan.LogOut, err = a.signedIn(ctx, id, l, true); err != nil {
			return err
		}
	}

	if !confirm(plan) {
		return ErrCancelled
	}

	if plan.LogOut {
		if err = a.logout(ctx, id); err != nil {
			return fmt.Errorf("%w: claude auth logout failed: %w", ErrCancelled, err)
		}
	}

	err = a.Store.Update(func(st *state.State) error {
		delete(st.Logins, id)
		if st.Default == id {
			st.Default = ""
		}
		return nil
	})
	if err != nil {
		return err
	}

	return os.RemoveAll(a.Store.Dir(id))
}

// logout runs claude auth logout on the sign-in stored for the login.
func (a *App) logout(ctx context.Context, id string) error {
	path, err := a.FindClaude()
	if err != nil {
		return err
	}
	return a.Logout(ctx, path, claude.StoredEnv(a.Env, a.Store.Dir(id)))
}

// Launch is how to start claude with a login.
type Launch struct {
	Path string   // the claude executable
	Env  []string // its environment: the login's credentials, nobody else's
	Name string   // the login
}

// Launch prepares a claude run with login name. Without a name it is the
// login of the claude this runs in, when there is one (CC_LOGIN_ID), else
// the default login.
func (a *App) Launch(name string) (Launch, error) {
	depth, _ := strconv.Atoi(lookup(a.Env, "CC_LOGIN_DEPTH"))
	if depth >= maxDepth {
		return Launch{}, fmt.Errorf("cc-login started itself %d times: the claude it finds leads back to it; set CC_LOGIN_CLAUDE to the real claude", depth)
	}

	st, err := a.Store.Load()
	if err != nil {
		return Launch{}, err
	}

	id, err := a.launchID(st, name)
	if err != nil {
		return Launch{}, err
	}
	l := st.Logins[id]

	path, err := a.FindClaude()
	if err != nil {
		return Launch{}, err
	}

	env, err := claude.Env(a.Env, a.Store.Dir(id), l.Kind)
	if err != nil {
		return Launch{}, err
	}
	env = append(env, "CC_LOGIN_ID="+id, "CC_LOGIN_DEPTH="+strconv.Itoa(depth+1))

	return Launch{Path: path, Env: env, Name: l.Name}, nil
}

// launchID picks the login for Launch.
func (a *App) launchID(st *state.State, name string) (string, error) {
	if name != "" {
		id, _, err := st.ByName(name)
		return id, err
	}

	if id := lookup(a.Env, "CC_LOGIN_ID"); st.Logins[id] != nil {
		return id, nil
	}

	if st.Logins[st.Default] == nil {
		return "", ErrNoDefault
	}
	return st.Default, nil
}

// lookup is the value of variable key in env.
func lookup(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if len(env[i]) > len(key) && env[i][len(key)] == '=' && env[i][:len(key)] == key {
			return env[i][len(key)+1:]
		}
	}
	return ""
}
