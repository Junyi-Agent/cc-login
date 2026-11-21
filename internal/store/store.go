// Package store reads the login layout that cc-login keeps on disk:
//
//	ROOT/.dirs/ID/            a credentials directory
//	ROOT/.dirs/ID/api-key     the key of an API key login
//	ROOT/.dirs/ID/oauth-token the token of a token login (claude setup-token)
//	ROOT/NAME                 login NAME: a link to .dirs/ID (or, in old setups, a directory)
//	ROOT/NAME.json            the account recorded for login NAME
//	ROOT/current              the name of the default login; empty when there is none
//	ROOT/.pending             the ID of the directory used while no login is the default
//
// ROOT is $XDG_DATA_HOME/claude-logins, or ~/.local/share/claude-logins.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// Store is one cc-login data directory.
type Store struct {
	Root string
}

// DefaultRoot is the data directory that the zsh version uses:
// ${XDG_DATA_HOME:-$HOME/.local/share}/claude-logins.
func DefaultRoot() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}

	return filepath.Join(base, "claude-logins")
}

// New returns the store at CC_LOGIN_ROOT when set, else at DefaultRoot.
func New() *Store {
	if r := os.Getenv("CC_LOGIN_ROOT"); r != "" {
		return &Store{Root: r}
	}
	return &Store{Root: DefaultRoot()}
}

// SetUp reports whether cc-login is set up on this machine: ROOT/current exists.
func (s *Store) SetUp() bool {
	_, err := os.Lstat(filepath.Join(s.Root, "current"))
	return err == nil
}

// Dir returns the credentials directory of login name. A link's path is
// rebuilt from its target's ID, so it is the same string at every call and
// Claude Code finds the same Keychain item (the item name hashes the path).
func (s *Store) Dir(name string) (string, bool) {
	if name == "" || strings.Contains(name, "/") || strings.HasPrefix(name, ".") {
		return "", false
	}

	// A link: rebuild the path from the target's ID.
	link := filepath.Join(s.Root, name)
	dir := link
	if fi, err := os.Lstat(link); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(link)
		if err != nil {
			return "", false
		}
		dir = filepath.Join(s.Root, ".dirs", filepath.Base(target))
	}

	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return "", false
	}
	return dir, true
}

// Names lists the logins: the entries of ROOT that do not start with '.' and
// are directories or links to directories, sorted.
func (s *Store) Names() []string {
	ents, err := os.ReadDir(s.Root)
	if err != nil {
		return nil
	}

	var names []string
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		if fi, err := os.Stat(filepath.Join(s.Root, n)); err == nil && fi.IsDir() {
			names = append(names, n)
		}
	}

	sort.Strings(names)
	return names
}

// Default returns the name of the default login, or false when there is none.
func (s *Store) Default() (string, bool) {
	name, err := ReadTrimmed(filepath.Join(s.Root, "current"))
	if err != nil {
		return "", false
	}

	if _, ok := s.Dir(name); !ok {
		return "", false
	}

	return name, true
}

// RecordPath is the account record of login name.
func (s *Store) RecordPath(name string) string {
	return filepath.Join(s.Root, name+".json")
}

// Record reads the account record of login name.
func (s *Store) Record(name string) (map[string]any, error) {
	b, err := os.ReadFile(s.RecordPath(name))
	if err != nil {
		return nil, err
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Kind is what a credentials directory holds.
type Kind int

// The kinds of credentials directory.
const (
	KindSignIn Kind = iota // a sign-in in .credentials.json or the Keychain
	KindKey                // an Anthropic API key in api-key
	KindToken              // a token from claude setup-token in oauth-token
)

// DirKind tells an API key login and a token login from a sign-in.
func DirKind(dir string) Kind {
	switch {
	case Readable(filepath.Join(dir, "api-key")):
		return KindKey
	case Readable(filepath.Join(dir, "oauth-token")):
		return KindToken
	}
	return KindSignIn
}

// Readable is the shell's [[ -r path ]].
func Readable(path string) bool {
	return syscall.Access(path, 4) == nil
}

// ReadTrimmed reads a file the way $(<file) does: without trailing newlines.
func ReadTrimmed(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(string(b), "\n"), nil
}
