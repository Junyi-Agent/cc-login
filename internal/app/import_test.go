package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/seal"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// exportedFile is the export file of exportApp's logins, ci the default.
func exportedFile(t *testing.T) string {
	t.Helper()

	file := filepath.Join(t.TempDir(), "logins.cc")
	if _, err := exportApp(t).Export(file, nil, &prompts{secrets: []string{passphrase, passphrase}}); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestImportOnANewMachine(t *testing.T) {
	file := exportedFile(t)
	a := newApp(t, map[string]*state.Login{}, "", &fake{status: map[string]claude.AuthInfo{}})

	res, err := a.Import(file, &prompts{secrets: []string{passphrase}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Names, []string{"api", "ci"}) || res.Skipped != nil || res.Default != "ci" {
		t.Errorf("result = %+v", res)
	}

	st := load(t, a)
	if st.Logins[st.Default].Name != "ci" {
		t.Errorf("the default is %q; want the file's, ci", st.Logins[st.Default].Name)
	}

	id, l, err := st.ByName("api")
	if err != nil || l.Kind != state.APIKey {
		t.Fatalf("api = %+v, %v", l, err)
	}
	if k, secretErr := creds.Secret(a.Store.Dir(id), state.APIKey); secretErr != nil || k != goodKey {
		t.Error("api's key did not come over")
	}
	fi, err := os.Stat(filepath.Join(a.Store.Dir(id), creds.APIKeyFile))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("key file: %v, %v; want 0600", fi, err)
	}
}

// A name this machine has is skipped; the default stays.
func TestImportSkipsNames(t *testing.T) {
	file := exportedFile(t)
	f := &fake{status: map[string]claude.AuthInfo{}}
	a := newApp(t, map[string]*state.Login{"x1": {Name: "ci", Kind: state.SignIn}}, "x1", f)

	res, err := a.Import(file, &prompts{secrets: []string{passphrase}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Names, []string{"api"}) || !slices.Equal(res.Skipped, []string{"ci"}) || res.Default != "" {
		t.Errorf("result = %+v", res)
	}

	st := load(t, a)
	if st.Default != "x1" || st.Logins["x1"].Kind != state.SignIn || len(st.Logins) != 2 {
		t.Errorf("state = %+v", st)
	}
}

// Without the file's default, the first login becomes the default.
func TestImportFirstIsDefault(t *testing.T) {
	file := filepath.Join(t.TempDir(), "logins.cc")
	if _, err := exportApp(t).Export(file, []string{"api"}, &prompts{secrets: []string{passphrase, passphrase}}); err != nil {
		t.Fatal(err)
	}
	a := newApp(t, map[string]*state.Login{}, "", &fake{status: map[string]claude.AuthInfo{}})

	res, err := a.Import(file, &prompts{secrets: []string{passphrase}})
	if err != nil || res.Default != "api" {
		t.Errorf("Import = %+v, %v; want api the default", res, err)
	}
}

func TestImportRefuses(t *testing.T) {
	good := exportedFile(t)

	cases := []struct {
		name  string
		file  func(t *testing.T) string
		input []string
		want  string
	}{
		{"a wrong passphrase", func(*testing.T) string { return good }, []string{"wrong horse"}, "wrong passphrase"},
		{"no passphrase", func(*testing.T) string { return good }, nil, "no input"},
		{"no file", func(t *testing.T) string { return filepath.Join(t.TempDir(), "nope") }, []string{passphrase}, "no such file"},
		{"a sign-in", sealedWith(`{"version":1,"logins":[{"name":"w","kind":"sign-in","secret":"sk-ant-x-12345"}]}`), []string{passphrase}, `kind "sign-in"`},
		{"a bad name", sealedWith(`{"version":1,"logins":[{"name":"../x","kind":"api-key","secret":"sk-ant-api-12345"}]}`), []string{passphrase}, "invalid login name"},
		{"a bad token", sealedWith(`{"version":1,"logins":[{"name":"t","kind":"token","secret":"sk-ant-api-12345"}]}`), []string{passphrase}, "no sk-ant-oat"},
		{"another version", sealedWith(`{"version":2,"logins":[]}`), []string{passphrase}, "version 2"},
	}

	for _, tc := range cases {
		a := newApp(t, map[string]*state.Login{}, "", &fake{status: map[string]claude.AuthInfo{}})

		_, err := a.Import(tc.file(t), &prompts{secrets: tc.input})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Import = %v; want %q", tc.name, err, tc.want)
		}
		if err != nil && strings.Contains(err.Error(), "12345") {
			t.Errorf("%s: the error shows the secret", tc.name)
		}

		if st := load(t, a); len(st.Logins) != 0 {
			t.Errorf("%s: imported %d logins", tc.name, len(st.Logins))
		}
		if dirs, _ := os.ReadDir(filepath.Join(a.Store.Root, ".dirs")); len(dirs) != 0 {
			t.Errorf("%s: left %d directories", tc.name, len(dirs))
		}
	}
}

// sealedWith makes an export file of the content plain.
func sealedWith(plain string) func(t *testing.T) string {
	return func(t *testing.T) string {
		t.Helper()

		if !json.Valid([]byte(plain)) {
			t.Fatalf("not JSON: %s", plain)
		}
		b, err := seal.Seal(passphrase, []byte(plain))
		if err != nil {
			t.Fatal(err)
		}

		file := filepath.Join(t.TempDir(), "x.cc")
		if err := os.WriteFile(file, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return file
	}
}
