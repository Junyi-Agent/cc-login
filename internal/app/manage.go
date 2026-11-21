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

// authStatus runs claude auth status with env.
func (a *App) authStatus(ctx context.Context, env []string) (claude.Status, error) {
	path, err := a.FindClaude()
	if err != nil {
		return claude.Status{}, err
	}
	return a.AuthStatus(ctx, path, env)
}

// storedEnv is the environment in which claude sees only the sign-in stored
// for the login with that ID, not the login's key or token.
func (a *App) storedEnv(id string) []string {
	return claude.StoredEnv(a.Env, a.Store.Dir(id))
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

	env, err := claude.Env(a.Env, a.Store.Dir(id), l.Kind)
	if err != nil {
		return err
	}

	cs, err := a.authStatus(ctx, env)
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
		cs, err := a.authStatus(ctx, a.storedEnv(id))
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
		if err := a.logout(ctx, p.ID); err != nil {
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

// logout runs claude auth logout on the sign-in stored for the login.
func (a *App) logout(ctx context.Context, id string) error {
	path, err := a.FindClaude()
	if err != nil {
		return err
	}
	return a.Logout(ctx, path, a.storedEnv(id))
}

// Launch is how to start claude with a login.
type Launch struct {
	Path string   // the claude executable
	Env  []string // its environment: the login's credentials, nobody else's
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
	a.Log.Debug("launch", "login", l.Name, "id", id, "kind", l.Kind, "claude", path, "depth", depth+1)

	return Launch{Path: path, Env: env}, nil
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
