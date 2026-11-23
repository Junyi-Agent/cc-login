package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoadEmptyRoot(t *testing.T) {
	s := &Store{Root: filepath.Join(t.TempDir(), "none")}

	st, err := s.Load()
	if err != nil || len(st.Logins) != 0 || st.Version != Version {
		t.Fatalf("Load = %+v, %v; want no logins", st, err)
	}
}

func TestUpdateRoundTrip(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	saved := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)

	err := s.Update(func(st *State) error {
		st.Logins["A"] = &Login{Name: "work", Kind: Token, Account: "me@example.com", TokenExpiresAt: &saved}
		st.Default = "A"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(filepath.Join(s.Root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("state.json mode = %v; want 0600", fi.Mode().Perm())
	}

	leftovers, _ := filepath.Glob(filepath.Join(s.Root, ".state.json-*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}

	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	l := st.Logins["A"]
	if st.Default != "A" || l.Name != "work" || l.Kind != Token || !l.TokenExpiresAt.Equal(saved) {
		t.Errorf("loaded %+v, %+v", st, l)
	}
}

// A failing fn leaves state.json as it was.
func TestUpdateFailing(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	if err := s.Update(func(st *State) error { st.Default = "A"; return nil }); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("boom")
	err := s.Update(func(st *State) error { st.Default = "B"; return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("Update = %v; want boom", err)
	}

	st, _ := s.Load()
	if st.Default != "A" {
		t.Errorf("Default = %q; want A", st.Default)
	}
}

func TestLoadVersion(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	if err := os.WriteFile(filepath.Join(s.Root, "state.json"), []byte(`{"version": 2, "logins": {}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Load(); err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Errorf("Load = %v; want a version error", err)
	}
}

// Updates from several goroutines all land: the lock serializes them.
func TestUpdateConcurrent(t *testing.T) {
	s := &Store{Root: t.TempDir()}

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Update(func(st *State) error {
				id := fmt.Sprintf("id%02d", i)
				st.Logins[id] = &Login{Name: "n" + id, Kind: SignIn}
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	st, err := s.Load()
	if err != nil || len(st.Logins) != 20 {
		t.Fatalf("%d logins, %v; want 20", len(st.Logins), err)
	}
}

func TestCheckName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"work", true},
		{"_x", true},
		{"a.b-c_1", true},
		{"", false},
		{".hidden", false},
		{"-x", false},
		{"a/b", false},
		{"a b", false},
		{"sk-ant", true},
		{"sk-antx", true},
		{"my-sk-ant-x", true},
		{"sk-ant-api03-pasted", false},
		{"sk-ant-oat01-pasted", false},
		{"SK-ANT-API03-PASTED", false},
		{"Sk-Ant-x", false},
	}

	for _, tc := range cases {
		err := CheckName(tc.name)
		if (err == nil) != tc.ok {
			t.Errorf("CheckName(%q) = %v; want ok %v", tc.name, err, tc.ok)
		}

		if err != nil && tc.name != "" && strings.Contains(err.Error(), tc.name) {
			t.Errorf("CheckName(%q) = %v; the error shows the name", tc.name, err)
		}
	}
}

// A name that looks like a key is refused as one.
func TestCheckNameKey(t *testing.T) {
	err := CheckName("sk-ant-api03-pasted")
	if err == nil || !strings.Contains(err.Error(), "key or token") {
		t.Errorf("CheckName(sk-ant-api03-pasted) = %v; want it to say it looks like a key", err)
	}
}

func TestByNameAndOrder(t *testing.T) {
	st := &State{Logins: map[string]*Login{
		"Z": {Name: "alpha"},
		"A": {Name: "gamma"},
		"M": {Name: "beta"},
	}}

	if got := st.IDs(); !slices.Equal(got, []string{"Z", "M", "A"}) {
		t.Errorf("IDs = %v; want sorted by name", got)
	}
	if got := st.Names(); !slices.Equal(got, []string{"alpha", "beta", "gamma"}) {
		t.Errorf("Names = %v", got)
	}

	id, l, err := st.ByName("beta")
	if err != nil || id != "M" || l.Name != "beta" {
		t.Errorf("ByName(beta) = %q, %v, %v", id, l, err)
	}

	_, _, err = st.ByName("sk-ant-api03-pasted")
	if !errors.Is(err, ErrNoLogin) || strings.Contains(err.Error(), "sk-ant") {
		t.Errorf("ByName(sk-ant-api03-pasted) = %v; want ErrNoLogin, without the name", err)
	}
}

func TestDir(t *testing.T) {
	s := &Store{Root: "/data/claude-logins"}
	if got := s.Dir("k3x9"); got != "/data/claude-logins/.dirs/k3x9" {
		t.Errorf("Dir = %q", got)
	}
}

func TestNewRoot(t *testing.T) {
	t.Setenv("CC_LOGIN_ROOT", "/x/root")
	if got := New().Root; got != "/x/root" {
		t.Errorf("New().Root = %q", got)
	}

	t.Setenv("CC_LOGIN_ROOT", "")
	t.Setenv("XDG_DATA_HOME", "/xdg")
	if got := New().Root; got != "/xdg/claude-logins" {
		t.Errorf("New().Root = %q", got)
	}
}
