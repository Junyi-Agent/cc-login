package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// layout makes a store with: a (link to .dirs/AAA), b (link to .dirs/BBB,
// an API key login), old (a plain directory), gone (a link that leads
// nowhere), a.json, current and .pending.
func layout(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, d := range []string{".dirs/AAA", ".dirs/BBB", "old"} {
		must(os.MkdirAll(filepath.Join(root, d), 0o700))
	}

	must(os.Symlink(".dirs/AAA", filepath.Join(root, "a")))
	must(os.Symlink(".dirs/BBB", filepath.Join(root, "b")))
	must(os.Symlink(".dirs/NOPE", filepath.Join(root, "gone")))

	must(os.WriteFile(filepath.Join(root, ".dirs/BBB/api-key"), []byte("sk-ant-x\n"), 0o600))
	must(os.WriteFile(filepath.Join(root, "a.json"), []byte(`{"email":"a@x"}`), 0o600))
	must(os.WriteFile(filepath.Join(root, "current"), []byte("a\n"), 0o600))
	must(os.WriteFile(filepath.Join(root, ".pending"), []byte("CCC\n"), 0o600))

	return &Store{Root: root}
}

func TestNames(t *testing.T) {
	s := layout(t)
	if got, want := s.Names(), []string{"a", "b", "old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
}

func TestDir(t *testing.T) {
	s := layout(t)
	cases := map[string]string{
		"a":       filepath.Join(s.Root, ".dirs", "AAA"),
		"old":     filepath.Join(s.Root, "old"),
		"gone":    "",
		"missing": "",
		"":        "",
		".dirs":   "",
		"a/b":     "",
		"a.json":  "",
	}

	for name, want := range cases {
		got, ok := s.Dir(name)
		if ok != (want != "") || got != want {
			t.Errorf("Dir(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
}

func TestDirRebuildsPathFromID(t *testing.T) {
	// A link written with an absolute target still gives ROOT/.dirs/ID, the
	// path string that names the Keychain item.
	s := layout(t)
	abs := filepath.Join(s.Root, "abs")
	if err := os.Symlink(filepath.Join(s.Root, ".dirs", "AAA"), abs); err != nil {
		t.Fatal(err)
	}

	if got, _ := s.Dir("abs"); got != filepath.Join(s.Root, ".dirs", "AAA") {
		t.Fatalf("Dir(abs) = %q", got)
	}
}

func TestDefault(t *testing.T) {
	s := layout(t)
	if name, ok := s.Default(); !ok || name != "a" {
		t.Fatalf("Default = %q, %v", name, ok)
	}

	if err := os.WriteFile(filepath.Join(s.Root, "current"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Default(); ok {
		t.Fatal("empty current: want no default")
	}

	if err := os.WriteFile(filepath.Join(s.Root, "current"), []byte("gone"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Default(); ok {
		t.Fatal("current naming a broken link: want no default")
	}
}

func TestDirKind(t *testing.T) {
	s := layout(t)
	a, _ := s.Dir("a")
	b, _ := s.Dir("b")
	if DirKind(a) != KindSignIn || DirKind(b) != KindKey {
		t.Fatalf("kinds: a=%v b=%v", DirKind(a), DirKind(b))
	}

	if err := os.WriteFile(filepath.Join(a, "oauth-token"), []byte("sk-ant-oat-x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if DirKind(a) != KindToken {
		t.Fatal("want token login")
	}
}

func TestRecord(t *testing.T) {
	s := layout(t)
	rec, err := s.Record("a")
	if err != nil || rec["email"] != "a@x" {
		t.Fatalf("Record = %v, %v", rec, err)
	}

	if _, err := s.Record("b"); err == nil {
		t.Fatal("b has no record")
	}
}
