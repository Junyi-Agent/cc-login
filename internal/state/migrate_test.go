package state

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// legacy writes a zsh layout under root: one file per entry of files
// (relative path → content), dirs as credentials directories, links as
// NAME → target.
func legacy(t *testing.T, root string, dirs []string, links, files map[string]string) {
	t.Helper()

	for _, id := range dirs {
		if err := os.MkdirAll(filepath.Join(root, ".dirs", id), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrate(t *testing.T) {
	root := t.TempDir()
	s := &Store{Root: root}
	now := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)

	legacy(t, root,
		[]string{"AAAAAAAA", "KKKKKKKK", "TTTTTTTT", "PPPPPPPP"},
		map[string]string{
			"work":   ".dirs/AAAAAAAA",
			"ci":     ".dirs/KKKKKKKK",
			"team":   ".dirs/TTTTTTTT",
			"broken": ".dirs/GONE0000",
		},
		map[string]string{ //nolint:gosec // G101: fake key and token values
			"work.json": `{"email":"me@example.com","orgId":"org-1","orgName":"Example","subscriptionType":"max"}`,
			"ci.json":   `{"authMethod":"api_key","apiKeyHint":"…wxyz"}`,
			"team.json": `{"authMethod":"oauth_token","email":"me@example.com","loginExpiresAt":"2027-11-20T07:22:44Z","tokenSavedAt":"2026-11-20T07:22:44Z"}`,
			"current":   "work\n",
			".pending":  "PPPPPPPP\n",

			".dirs/KKKKKKKK/api-key":           "sk-ant-api-example\n",
			".dirs/TTTTTTTT/oauth-token":       "sk-ant-oat-example\n",
			".dirs/PPPPPPPP/.credentials.json": `{}`,
		})

	m, err := s.Migrate(now)
	if err != nil {
		t.Fatal(err)
	}

	if m.Already || m.Default != "work" || !slices.Equal(m.Logins, []string{"ci", "default", "team", "work"}) {
		t.Errorf("migration = %+v", m)
	}
	if len(m.Notes) != 2 {
		t.Errorf("notes = %q; want the broken link and the pending login", m.Notes)
	}

	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}

	work := st.Logins["AAAAAAAA"]
	if st.Default != "AAAAAAAA" || work.Name != "work" || work.Kind != SignIn ||
		work.Account != "me@example.com" || work.OrgID != "org-1" || work.OrgName != "Example" || work.Plan != "max" {
		t.Errorf("work = %+v, default %q", work, st.Default)
	}

	ci := st.Logins["KKKKKKKK"]
	if ci.Kind != APIKey || ci.KeyHint != "wxyz" {
		t.Errorf("ci = %+v", ci)
	}

	team := st.Logins["TTTTTTTT"]
	expires := time.Date(2027, 11, 20, 7, 22, 44, 0, time.UTC)
	if team.Kind != Token || team.TokenExpiresAt == nil || !team.TokenExpiresAt.Equal(expires) || team.TokenSavedAt == nil {
		t.Errorf("team = %+v", team)
	}

	if p := st.Logins["PPPPPPPP"]; p == nil || p.Name != "default" || p.Kind != SignIn {
		t.Errorf("pending login = %+v", p)
	}

	// The credentials stay where they are.
	for _, f := range []string{".dirs/KKKKKKKK/api-key", ".dirs/TTTTTTTT/oauth-token", ".dirs/PPPPPPPP/.credentials.json"} {
		if _, errStat := os.Stat(filepath.Join(root, f)); errStat != nil {
			t.Errorf("%s: %v", f, errStat)
		}
	}

	// The old entries are in the backup, and only there.
	if m.Backup != filepath.Join(root, ".legacy-20261121-080000") {
		t.Errorf("backup = %q", m.Backup)
	}
	for _, name := range []string{"work", "ci", "team", "broken", "work.json", "ci.json", "team.json", "current", ".pending"} {
		if _, errB := os.Lstat(filepath.Join(m.Backup, name)); errB != nil {
			t.Errorf("backup lacks %s: %v", name, errB)
		}
		if _, errR := os.Lstat(filepath.Join(root, name)); errR == nil {
			t.Errorf("%s is still in the root", name)
		}
	}

	again, err := s.Migrate(now.Add(time.Minute))
	if err != nil || !again.Already {
		t.Errorf("second run = %+v, %v; want Already", again, err)
	}
}

// The pending login takes default-2 when default is taken.
func TestMigratePendingName(t *testing.T) {
	root := t.TempDir()
	legacy(t, root,
		[]string{"AAAAAAAA", "PPPPPPPP"},
		map[string]string{"default": ".dirs/AAAAAAAA"},
		map[string]string{
			"current":                          "\n",
			".pending":                         "PPPPPPPP\n",
			".dirs/PPPPPPPP/.credentials.json": `{}`,
		})

	m, err := (&Store{Root: root}).Migrate(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// With no default before, the pending login becomes the default.
	if !slices.Equal(m.Logins, []string{"default", "default-2"}) || m.Default != "default-2" {
		t.Errorf("migration = %+v", m)
	}
}

// A pending directory without credentials is no login.
func TestMigratePendingEmpty(t *testing.T) {
	root := t.TempDir()
	legacy(t, root,
		[]string{"AAAAAAAA", "PPPPPPPP"},
		map[string]string{"work": ".dirs/AAAAAAAA"},
		map[string]string{"current": "work\n", ".pending": "PPPPPPPP\n"})

	m, err := (&Store{Root: root}).Migrate(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(m.Logins, []string{"work"}) || len(m.Notes) != 0 {
		t.Errorf("migration = %+v", m)
	}
}

// A login that is a real directory stops the migration before it writes
// anything.
func TestMigrateRealDirectory(t *testing.T) {
	root := t.TempDir()
	legacy(t, root,
		[]string{"AAAAAAAA"},
		map[string]string{"work": ".dirs/AAAAAAAA"},
		map[string]string{"current": "work\n"})
	if err := os.Mkdir(filepath.Join(root, "old"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := (&Store{Root: root}).Migrate(time.Now())
	if err == nil || !strings.Contains(err.Error(), "login old is a directory") {
		t.Fatalf("Migrate = %v; want the directory error", err)
	}

	if _, err := os.Stat(filepath.Join(root, "state.json")); err == nil {
		t.Error("state.json was written")
	}
	for _, name := range []string{"work", "current", "old"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
			t.Errorf("%s moved: %v", name, err)
		}
	}
}

func TestKindOf(t *testing.T) {
	dir := t.TempDir()
	if KindOf(dir) != SignIn {
		t.Error("empty dir: want sign-in")
	}

	if err := os.WriteFile(filepath.Join(dir, TokenFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if KindOf(dir) != Token {
		t.Error("oauth-token: want token")
	}

	if err := os.WriteFile(filepath.Join(dir, APIKeyFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if KindOf(dir) != APIKey {
		t.Error("api-key: want api-key, which wins over a token")
	}
}
