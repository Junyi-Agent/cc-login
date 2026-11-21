package app

import (
	"fmt"
	"strconv"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// maxDepth is how many cc-logins may run inside each other: a claude can
// start a claude (claude -p, hooks), but a claude link that leads back to
// cc-login without a real claude between them loops.
const maxDepth = 10

// Exec is the program cc-login run replaces itself with.
type Exec struct {
	Path string   // the claude executable
	Env  []string // its environment: the login's credentials, nobody else's
}

// Run prepares a claude run with login name; the caller execs it. Without a
// name it is the login of the claude this runs in, when there is one
// (CC_LOGIN_ID), else the default login.
func (a *App) Run(name string) (Exec, error) {
	depth, _ := strconv.Atoi(lookup(a.Env, "CC_LOGIN_DEPTH"))
	if depth >= maxDepth {
		return Exec{}, fmt.Errorf("cc-login started itself %d times: the claude it finds leads back to it; set CC_LOGIN_CLAUDE to the real claude", depth)
	}

	st, err := a.Store.Load()
	if err != nil {
		return Exec{}, err
	}

	id, err := a.runID(st, name)
	if err != nil {
		return Exec{}, err
	}
	l := st.Logins[id]

	path, err := a.Claude.Path()
	if err != nil {
		return Exec{}, err
	}

	env, err := a.env(id, l.Kind)
	if err != nil {
		return Exec{}, err
	}
	env = append(env, "CC_LOGIN_ID="+id, "CC_LOGIN_DEPTH="+strconv.Itoa(depth+1))
	a.Log.Debug("run", "login", l.Name, "id", id, "kind", l.Kind, "claude", path, "depth", depth+1)

	return Exec{Path: path, Env: env}, nil
}

// runID picks the login for Run.
func (a *App) runID(st *state.State, name string) (string, error) {
	if name != "" {
		id, _, err := st.ByName(name)
		return id, err
	}

	if id := lookup(a.Env, "CC_LOGIN_ID"); st.Logins[id] != nil {
		return id, nil
	}

	switch {
	case len(st.Logins) == 0:
		return "", ErrNoLogins
	case st.Logins[st.Default] == nil:
		return "", ErrNoDefault
	}
	return st.Default, nil
}
