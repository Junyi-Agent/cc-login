package app

import (
	"fmt"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

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
