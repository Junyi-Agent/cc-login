package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The zsh version's layout: ROOT/NAME -> .dirs/ID links, ROOT/NAME.json
// records, ROOT/current naming the default, ROOT/.pending holding the ID of
// the directory used while there was no default.

// hasLegacy reports whether the root has the zsh version's layout.
func (s *Store) hasLegacy() bool {
	if _, err := os.Lstat(filepath.Join(s.Root, "current")); err == nil {
		return true
	}

	ents, _ := os.ReadDir(s.Root)
	for _, e := range ents {
		if e.Type()&os.ModeSymlink != 0 && !strings.HasPrefix(e.Name(), ".") {
			return true
		}
	}
	return false
}

// Migration is what Migrate did.
type Migration struct {
	Already bool     // state.json was there: nothing changed
	Logins  []string // the names of the logins in state.json
	Default string   // the default login, "" for none
	Notes   []string // entries left out, and the login made from .pending
	Backup  string   // where the old links and records went
}

// legacyRecord is the part of a zsh record (NAME.json) that state.json keeps.
type legacyRecord struct {
	Email            string `json:"email"`
	OrgID            string `json:"orgId"`
	OrgName          string `json:"orgName"`
	SubscriptionType string `json:"subscriptionType"`
	APIKeyHint       string `json:"apiKeyHint"`
	LoginExpiresAt   string `json:"loginExpiresAt"`
	TokenSavedAt     string `json:"tokenSavedAt"`
}

// Migrate converts the zsh version's layout into state.json. The
// credentials directories stay where they are; the links, records, current
// and .pending move to ROOT/.legacy-<time>/. With state.json already there
// it does nothing.
func (s *Store) Migrate(now time.Time) (Migration, error) {
	var m Migration

	unlock, err := s.lock()
	if err != nil {
		return m, err
	}
	defer unlock()

	if _, statErr := os.Stat(s.path()); statErr == nil {
		m.Already = true
		return m, nil
	}

	st, moved, err := s.readLegacy(&m, now)
	if err != nil {
		return m, err
	}

	// state.json first: once it is there, the old files are only a backup.
	if err := s.save(st); err != nil {
		return m, err
	}
	for _, id := range st.IDs() {
		m.Logins = append(m.Logins, st.Logins[id].Name)
	}
	if l := st.Logins[st.Default]; l != nil {
		m.Default = l.Name
	}

	m.Backup = filepath.Join(s.Root, ".legacy-"+now.UTC().Format("20060102-150405"))
	if err := os.Mkdir(m.Backup, 0o700); err != nil {
		return m, err
	}
	for _, name := range moved {
		if err := os.Rename(filepath.Join(s.Root, name), filepath.Join(m.Backup, name)); err != nil {
			return m, fmt.Errorf("state.json is written, but moving %s into %s failed: %w", name, m.Backup, err)
		}
	}

	return m, nil
}

// readLegacy builds the state from the zsh layout, and lists the entries
// of the root that it read and that move to the backup.
func (s *Store) readLegacy(m *Migration, now time.Time) (*State, []string, error) {
	st := &State{Version: Version, Logins: map[string]*Login{}}
	var moved []string

	ents, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, nil, err
	}

	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "current" || strings.HasSuffix(name, ".json") {
			continue
		}

		// A login whose credentials are ROOT/NAME itself cannot move without
		// signing it out; the zsh version's rename moves it into .dirs first.
		if e.IsDir() {
			return nil, nil, fmt.Errorf("login %s is a directory, not a link into .dirs; move it with the zsh version (cc-login rename %s NEW), then migrate", name, name)
		}
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}

		id, err := s.linkID(name)
		if err != nil {
			m.Notes = append(m.Notes, err.Error())
			moved = append(moved, name)
			continue
		}

		st.Logins[id] = s.legacyLogin(name, id)
		moved = append(moved, name)
		if _, err := os.Lstat(filepath.Join(s.Root, name+".json")); err == nil {
			moved = append(moved, name+".json")
		}
	}

	if cur, err := readLine(filepath.Join(s.Root, "current")); err == nil {
		if id, _, err := st.ByName(cur); err == nil {
			st.Default = id
		}
		moved = append(moved, "current")
	}

	if pending, ok := s.pendingLogin(st, now); ok {
		m.Notes = append(m.Notes, "the directory claude signed in to while there was no default is now login "+pending)
	}
	if _, err := os.Lstat(filepath.Join(s.Root, ".pending")); err == nil {
		moved = append(moved, ".pending")
	}

	return st, moved, nil
}

// linkID is the ID of the directory that link ROOT/name points to.
func (s *Store) linkID(name string) (string, error) {
	target, err := filepath.EvalSymlinks(filepath.Join(s.Root, name))
	if err != nil {
		return "", fmt.Errorf("left out %s: its link points nowhere", name)
	}

	id := filepath.Base(target)
	if fi, err := os.Stat(s.Dir(id)); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("left out %s: it points outside %s", name, filepath.Join(s.Root, ".dirs"))
	}

	return id, nil
}

// legacyLogin is login name (directory ID) from its files and its record.
func (s *Store) legacyLogin(name, id string) *Login {
	dir := s.Dir(id)
	l := &Login{Name: name, Kind: KindOf(dir)}
	if fi, err := os.Stat(dir); err == nil {
		l.CreatedAt = fi.ModTime().UTC()
	}

	b, err := os.ReadFile(filepath.Join(s.Root, name+".json"))
	if err != nil {
		return l
	}
	var r legacyRecord
	if json.Unmarshal(b, &r) != nil {
		return l
	}

	l.Account, l.OrgID, l.OrgName, l.Plan = r.Email, r.OrgID, r.OrgName, r.SubscriptionType
	if l.Kind == APIKey {
		l.KeyHint = strings.TrimPrefix(r.APIKeyHint, "…")
	}
	if l.Kind == Token {
		l.TokenSavedAt = parseTime(r.TokenSavedAt)
		l.TokenExpiresAt = parseTime(r.LoginExpiresAt)
	}

	return l
}

// pendingLogin turns the .pending directory into a login named default,
// default-2, ..., when claude signed in there (it holds .credentials.json).
// A sign-in claude kept in the macOS Keychain is not seen.
func (s *Store) pendingLogin(st *State, now time.Time) (string, bool) {
	id, err := readLine(filepath.Join(s.Root, ".pending"))
	if err != nil || id == "" || strings.Contains(id, "/") || st.Logins[id] != nil {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(s.Dir(id), ".credentials.json")); err != nil {
		return "", false
	}

	name := "default"
	for n := 2; ; n++ {
		if _, _, err := st.ByName(name); err != nil {
			break
		}
		name = fmt.Sprintf("default-%d", n)
	}

	st.Logins[id] = &Login{Name: name, Kind: KindOf(s.Dir(id)), CreatedAt: now.UTC()}
	if st.Default == "" {
		st.Default = id
	}
	return name, true
}

// The files in a credentials directory that hold an API key or a token:
// file names, not secrets.
const (
	APIKeyFile = "api-key"     //nolint:gosec // G101: a file name
	TokenFile  = "oauth-token" //nolint:gosec // G101: a file name
)

// KindOf is the kind of the login in credentials directory dir: an api-key
// file makes an API key login, an oauth-token file a token login.
func KindOf(dir string) Kind {
	switch {
	case exists(filepath.Join(dir, APIKeyFile)):
		return APIKey
	case exists(filepath.Join(dir, TokenFile)):
		return Token
	}
	return SignIn
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readLine reads a one-line file without its trailing newlines.
func readLine(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\n"), nil
}

// parseTime parses an RFC 3339 time; nil when s is empty or not one.
func parseTime(s string) *time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}

	t = t.UTC()
	return &t
}
