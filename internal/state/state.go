// Package state keeps the logins of one machine: state.json and the
// credentials directories under the data root (ADR 0008).
package state

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"syscall"
	"time"
)

// Kind is how a login authenticates.
type Kind string

// The kinds of login.
const (
	SignIn Kind = "sign-in" // claude auth login; Claude Code stores the sign-in
	Token  Kind = "token"   // a token from claude setup-token, in the oauth-token file
	APIKey Kind = "api-key" // an Anthropic API key, in the api-key file
)

// Login is one login in state.json. The credentials directory is
// ROOT/.dirs/ID; ID is the key of the login in State.Logins.
type Login struct {
	Name           string     `json:"name"`
	Kind           Kind       `json:"kind"`
	Account        string     `json:"account,omitempty"`  // an email, as claude or the user reported it
	OrgID          string     `json:"org_id,omitempty"`   // the organization's UUID
	OrgName        string     `json:"org_name,omitempty"` // the organization's name
	Plan           string     `json:"plan,omitempty"`     // claude's subscriptionType: pro, max, team...
	KeyHint        string     `json:"key_hint,omitempty"` // an API key's last 4 characters
	CreatedAt      time.Time  `json:"created_at"`
	TokenSavedAt   *time.Time `json:"token_saved_at,omitempty"`   // a token login: when the token was saved
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"` // a token login: a year after it was saved
}

// State is state.json.
type State struct {
	Version int               `json:"version"`
	Default string            `json:"default,omitempty"` // the ID of the default login; "" for none
	Logins  map[string]*Login `json:"logins"`            // by ID
}

// Version is the state.json format this code writes.
const Version = 1

// Errors of the store.
var (
	// ErrLegacy: the data root has the zsh version's layout and no state.json.
	ErrLegacy = errors.New("the logins are in the zsh version's layout; convert them with: cc-login migrate")

	// ErrNoLogin: no login has that name.
	ErrNoLogin = errors.New("no such login")
)

// Store is the data root.
type Store struct {
	Root string
}

// DefaultRoot is $XDG_DATA_HOME/claude-logins, or ~/.local/share/claude-logins.
func DefaultRoot() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "claude-logins")
}

// New is the store at $CC_LOGIN_ROOT, or at DefaultRoot.
func New() *Store {
	if r := os.Getenv("CC_LOGIN_ROOT"); r != "" {
		return &Store{Root: r}
	}
	return &Store{Root: DefaultRoot()}
}

func (s *Store) path() string { return filepath.Join(s.Root, "state.json") }

// Dir is the credentials directory of the login with that ID. The string
// must stay the same for the life of the login: on macOS Claude Code names
// the Keychain item after its SHA-256.
func (s *Store) Dir(id string) string { return filepath.Join(s.Root, ".dirs", id) }

// Load reads state.json. A machine without one has no logins, unless the
// data root has the zsh version's layout: then it is ErrLegacy.
func (s *Store) Load() (*State, error) {
	b, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		if s.hasLegacy() {
			return nil, ErrLegacy
		}
		return &State{Version: Version, Logins: map[string]*Login{}}, nil
	}
	if err != nil {
		return nil, err
	}

	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path(), err)
	}
	if st.Version != Version {
		return nil, fmt.Errorf("%s has version %d; this cc-login reads version %d", s.path(), st.Version, Version)
	}
	if st.Logins == nil {
		st.Logins = map[string]*Login{}
	}

	return &st, nil
}

// Update changes state.json under the lock: it reads the state, calls fn,
// and, when fn succeeds, replaces the file atomically.
func (s *Store) Update(fn func(*State) error) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	st, err := s.Load()
	if err != nil {
		return err
	}

	if err := fn(st); err != nil {
		return err
	}

	return s.save(st)
}

// lock takes the exclusive lock on ROOT/.lock, making the root if needed.
func (s *Store) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Join(s.Root, ".dirs"), 0o700); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(filepath.Join(s.Root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock %s: %w", f.Name(), err)
	}

	return func() { _ = f.Close() }, nil // closing releases the lock
}

// save writes st to a temporary file in the root and renames it over
// state.json, so a reader sees the old or the new state, never a part.
func (s *Store) save(st *State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(s.Root, ".state-*.json") // 0600
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // fails harmlessly after the rename

	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), s.path())
}

// nameRE is the form of a login name.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)

// CheckName reports whether name can name a login.
func CheckName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("%q cannot name a login: use letters, digits, _ . and -, not first . or -", name)
	}
	return nil
}

// ByName finds the login named name and its ID.
func (st *State) ByName(name string) (string, *Login, error) {
	for id, l := range st.Logins {
		if l.Name == name {
			return id, l, nil
		}
	}
	return "", nil, fmt.Errorf("%w %q", ErrNoLogin, name)
}

// IDs are the IDs of the logins, sorted by name.
func (st *State) IDs() []string {
	ids := make([]string, 0, len(st.Logins))
	for id := range st.Logins {
		ids = append(ids, id)
	}

	slices.SortFunc(ids, func(a, b string) int {
		return cmp.Compare(st.Logins[a].Name, st.Logins[b].Name)
	})
	return ids
}

// Names are the names of the logins, sorted.
func (st *State) Names() []string {
	var names []string
	for _, id := range st.IDs() {
		names = append(names, st.Logins[id].Name)
	}
	return names
}
