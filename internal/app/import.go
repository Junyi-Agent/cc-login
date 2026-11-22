package app

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/seal"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Imported is what Import added, for the message.
type Imported struct {
	Names   []string // the logins added
	Skipped []string // logins of the file whose name this machine has
	Default string   // the login that became the default; "" when the default stayed
}

// Import adds the logins of an export file (ADR 0021), decrypted with the
// passphrase the user types. A name this machine has is skipped, never
// overwritten. Nothing is checked online: the logins come as they were.
func (a *App) Import(file string, p Prompter) (Imported, error) {
	sealed, err := os.ReadFile(file) //nolint:gosec // G304: the file the user named
	if err != nil {
		return Imported{}, err
	}

	pw, err := p.Secret("Passphrase of "+file, "The passphrase given to cc-login export; it shows as *.", func(string) error { return nil })
	if err != nil {
		return Imported{}, err
	}

	data, err := openExport(pw, sealed)
	if err != nil {
		return Imported{}, err
	}

	st, err := a.Store.Load()
	if err != nil {
		return Imported{}, err
	}

	var res Imported
	var todo []exported
	for _, e := range data.Logins {
		if _, _, err := st.ByName(e.Name); err == nil || named(todo, e.Name) {
			res.Skipped = append(res.Skipped, e.Name)
			continue
		}
		todo = append(todo, e)
	}
	slices.SortFunc(todo, func(x, y exported) int { return cmp.Compare(x.Name, y.Name) })

	a.Log.Debug("import", "file", file, "logins", len(todo), "skipped", len(res.Skipped))
	return a.addImported(todo, data.Default, res)
}

// openExport decrypts and checks an export file. One bad login refuses the
// whole file.
func openExport(passphrase string, sealed []byte) (*exportFile, error) {
	plain, err := seal.Open(passphrase, sealed)
	if err != nil {
		return nil, err
	}

	var data exportFile
	if err := json.Unmarshal(plain, &data); err != nil {
		return nil, fmt.Errorf("the export file: %w", err)
	}
	if data.Version != exportVersion {
		return nil, fmt.Errorf("an export file of version %d; this cc-login reads version %d", data.Version, exportVersion)
	}

	for i := range data.Logins {
		if err := checkExported(&data.Logins[i]); err != nil {
			return nil, fmt.Errorf("the export file, login %d: %w; nothing imported", i+1, err)
		}
	}
	return &data, nil
}

// checkExported checks one login of an export file: its name, its kind,
// and the form of its key or token. The error never shows the secret.
func checkExported(e *exported) error {
	if err := state.CheckName(e.Name); err != nil {
		return err
	}

	prefix := map[state.Kind]string{state.Token: "sk-ant-oat", state.APIKey: "sk-ant-"}[e.Kind]
	if prefix == "" {
		return fmt.Errorf("%s is of kind %q; an export file has token and api-key logins", e.Name, e.Kind)
	}

	if validPrefix(prefix)(e.Secret) != nil {
		return fmt.Errorf("%s has no %s... value", e.Name, prefix)
	}
	return nil
}

// named reports whether a login of logins is named name.
func named(logins []exported, name string) bool {
	for _, e := range logins {
		if e.Name == name {
			return true
		}
	}
	return false
}

// addImported writes the keys and tokens of logins into new credentials
// directories, then adds the logins to state.json at once. When this
// machine has no default, def (the file's default) becomes it if added,
// else the first login. On failure nothing is left behind.
func (a *App) addImported(logins []exported, def string, res Imported) (Imported, error) {
	ids := make([]string, 0, len(logins))
	undo := func() {
		for _, id := range ids {
			_ = os.RemoveAll(a.Store.Dir(id))
		}
	}

	for _, e := range logins {
		id, err := a.Store.NewDir()
		if err != nil {
			undo()
			return Imported{}, err
		}
		ids = append(ids, id)

		if err := creds.WriteSecret(a.Store.Dir(id), e.Kind, e.Secret); err != nil {
			undo()
			return Imported{}, err
		}
	}

	err := a.Store.Update(func(st *state.State) error {
		return putImported(st, ids, logins, def, &res)
	})
	if err != nil {
		undo()
		return Imported{}, err
	}
	return res, nil
}

// putImported adds logins, in the directories of ids, to st, and picks the
// default when st has none.
func putImported(st *state.State, ids []string, logins []exported, def string, res *Imported) error {
	for i, e := range logins {
		if _, _, err := st.ByName(e.Name); err == nil {
			return fmt.Errorf("login %s was added meanwhile; nothing imported", e.Name)
		}

		l := e.Login
		st.Logins[ids[i]] = &l
		res.Names = append(res.Names, e.Name)
	}

	if st.Logins[st.Default] != nil || len(ids) == 0 {
		return nil
	}

	st.Default = ids[0]
	for i, e := range logins {
		if e.Name == def {
			st.Default = ids[i]
		}
	}
	res.Default = st.Logins[st.Default].Name
	return nil
}
