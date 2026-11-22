package app

import (
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

func init() { seal.Iterations = 100_000 } // the fewest Open takes: fast tests

const passphrase = "correct horse" //nolint:gosec // G101: a fake

// exportApp is three with keys and tokens that import takes: ci's token is
// goodToken, api's key goodKey. ci is the default.
func exportApp(t *testing.T) *App {
	t.Helper()

	a, _ := three(t)
	write := map[string]string{"t1": creds.TokenFile, "k1": creds.APIKeyFile}
	secret := map[string]string{"t1": goodToken, "k1": goodKey}
	for id, file := range write {
		if err := os.WriteFile(filepath.Join(a.Store.Dir(id), file), []byte(secret[id]+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := a.Store.Update(func(st *state.State) error { st.Default = "t1"; return nil }); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestExport(t *testing.T) {
	a := exportApp(t)
	file := filepath.Join(t.TempDir(), "logins.cc")

	res, err := a.Export(file, nil, &prompts{secrets: []string{passphrase, passphrase}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Names, []string{"api", "ci"}) || !slices.Equal(res.SignIns, []string{"work"}) {
		t.Errorf("result = %+v", res)
	}

	fi, err := os.Stat(file)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the file: %v, %v; want 0600", fi, err)
	}
	b, _ := os.ReadFile(file)
	if strings.Contains(string(b), goodToken) || strings.Contains(string(b), `"secret"`) {
		t.Error("the file is not encrypted")
	}

	data, err := openExport(passphrase, b)
	if err != nil {
		t.Fatal(err)
	}
	if data.Default != "ci" || len(data.Logins) != 2 || data.Logins[1].Secret != goodToken || !data.ExportedAt.Equal(now) {
		t.Errorf("the content = %+v", data)
	}
}

func TestExportNamed(t *testing.T) {
	a := exportApp(t)
	file := filepath.Join(t.TempDir(), "logins.cc")

	res, err := a.Export(file, []string{"api", "api"}, &prompts{secrets: []string{passphrase, passphrase}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Names, []string{"api"}) || res.SignIns != nil {
		t.Errorf("result = %+v", res)
	}

	b, _ := os.ReadFile(file)
	if data, _ := openExport(passphrase, b); data == nil || data.Default != "" {
		t.Errorf("the default of the file is not exported: %+v", data)
	}
}

func TestExportRefuses(t *testing.T) {
	dir := t.TempDir()
	exists := filepath.Join(dir, "exists")
	if err := os.WriteFile(exists, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		file    string
		names   []string
		secrets []string
		want    string
	}{
		{"a sign-in", "a", []string{"work"}, nil, "is a sign-in"},
		{"no such login", "b", []string{"nope"}, nil, "no such login"},
		{"a file that exists", exists, nil, nil, "exists"},
		{"a short passphrase", "c", nil, []string{"short", "short"}, "at least 8"},
		{"two passphrases", "d", nil, []string{passphrase, "other horse"}, "not the same"},
		{"no passphrase", "e", nil, nil, "no input"},
	}

	for _, tc := range cases {
		a := exportApp(t)
		file := tc.file
		if !filepath.IsAbs(file) {
			file = filepath.Join(dir, file)
		}

		_, err := a.Export(file, tc.names, &prompts{secrets: tc.secrets})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Export = %v; want %q", tc.name, err, tc.want)
		}
		if b, _ := os.ReadFile(exists); string(b) != "mine" {
			t.Errorf("%s: Export changed a file that was there", tc.name)
		}
	}

	for _, f := range []string{"a", "b", "c", "d", "e"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("a failed export left %s", f)
		}
	}
}

// Only sign-in logins: nothing to export.
func TestExportNoneToExport(t *testing.T) {
	f := &fake{status: map[string]claude.AuthInfo{}}
	a := newApp(t, map[string]*state.Login{"a1": {Name: "work", Kind: state.SignIn}}, "a1", f)

	_, err := a.Export(filepath.Join(t.TempDir(), "x"), nil, &prompts{})
	if err == nil || !strings.Contains(err.Error(), "no token or API key logins") {
		t.Errorf("Export = %v", err)
	}
}
