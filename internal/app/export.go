package app

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/seal"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// exportFile is what an export file holds, encrypted (ADR 0021).
type exportFile struct {
	Version    int        `json:"version"`
	ExportedAt time.Time  `json:"exported_at"`
	Default    string     `json:"default,omitempty"` // the name of the default login, when it is exported
	Logins     []exported `json:"logins"`
}

// exported is one login in an export file: its fields in state.json, and
// its key or token.
type exported struct {
	state.Login
	Secret string `json:"secret"`
}

const exportVersion = 1

// minPassphrase is the length of the shortest passphrase export takes.
const minPassphrase = 8

// Exported is what Export wrote, for the message.
type Exported struct {
	Names   []string // the logins in the file
	SignIns []string // the sign-in logins left out
}

// Export writes the token and API key logins named in names, or all of them
// when names is empty, into a new file, encrypted with a passphrase the user
// types twice (ADR 0021). A sign-in login's credentials stay on this machine.
func (a *App) Export(file string, names []string, p Prompter) (Exported, error) {
	st, err := a.Store.Load()
	if err != nil {
		return Exported{}, err
	}

	data, res, err := a.collect(st, names)
	if err != nil {
		return Exported{}, err
	}

	if _, statErr := os.Lstat(file); statErr == nil {
		return Exported{}, fmt.Errorf("%s exists; choose another file, or remove it first", file)
	}

	passphrase, err := newPassphrase(file, p)
	if err != nil {
		return Exported{}, err
	}

	plain, err := json.Marshal(data)
	if err != nil {
		return Exported{}, err
	}
	sealed, err := seal.Seal(passphrase, plain)
	if err != nil {
		return Exported{}, err
	}

	a.Log.Debug("export", "file", file, "logins", len(res.Names), "sign-ins left out", len(res.SignIns))
	return res, writeNew(file, sealed)
}

// collect is the content of the export file of the logins named in names
// (all token and API key logins when empty), with their keys and tokens.
func (a *App) collect(st *state.State, names []string) (*exportFile, Exported, error) {
	ids, res, err := exportIDs(st, names)
	if err != nil {
		return nil, Exported{}, err
	}

	data := &exportFile{Version: exportVersion, ExportedAt: a.Now().UTC()}
	for _, id := range ids {
		l := st.Logins[id]

		secret, err := creds.Secret(a.Store.Dir(id), l.Kind)
		if err != nil {
			return nil, Exported{}, fmt.Errorf("login %s: %w", l.Name, err)
		}

		data.Logins = append(data.Logins, exported{Login: *l, Secret: secret})
		res.Names = append(res.Names, l.Name)
		if id == st.Default {
			data.Default = l.Name
		}
	}

	return data, res, nil
}

// exportIDs are the IDs of the logins to export, sorted by name, and the
// sign-in logins left out. A named sign-in login is an error.
func exportIDs(st *state.State, names []string) ([]string, Exported, error) {
	var res Exported

	if len(names) == 0 {
		var ids []string
		for _, id := range st.IDs() {
			if l := st.Logins[id]; l.Kind == state.SignIn {
				res.SignIns = append(res.SignIns, l.Name)
			} else {
				ids = append(ids, id)
			}
		}

		if len(ids) == 0 {
			return nil, res, errors.New("no token or API key logins to export; a sign-in login stays on this machine (add it on the other one with cc-login add NAME --sign-in)")
		}
		return ids, res, nil
	}

	var ids []string
	for _, name := range names {
		id, l, err := st.ByName(name)
		if err != nil {
			return nil, res, err
		}
		if l.Kind == state.SignIn {
			return nil, res, fmt.Errorf("login %s is a sign-in: its credentials stay on this machine; on the other one run cc-login add %s --sign-in", name, name)
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}

	slices.SortFunc(ids, func(x, y string) int { return cmp.Compare(st.Logins[x].Name, st.Logins[y].Name) })
	return ids, res, nil
}

// newPassphrase asks for the passphrase of a new export file, twice.
func newPassphrase(file string, p Prompter) (string, error) {
	long := func(s string) error {
		if len(s) < minPassphrase {
			return fmt.Errorf("use at least %d characters", minPassphrase)
		}
		return nil
	}

	pw, err := p.Secret("Passphrase for "+file, fmt.Sprintf("At least %d characters; import asks for it. It shows as *.", minPassphrase), long)
	if err != nil {
		return "", err
	}
	if err = long(pw); err != nil {
		return "", err
	}

	same := func(s string) error {
		if s != pw {
			return errors.New("not the same passphrase")
		}
		return nil
	}

	again, err := p.Secret("The passphrase again", "", same)
	if err != nil {
		return "", err
	}
	return pw, same(again)
}

// writeNew writes data into a new 0600 file; it fails when file exists.
func writeNew(file string, data []byte) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: the file the user named
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(file)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(file)
		return err
	}
	return nil
}
