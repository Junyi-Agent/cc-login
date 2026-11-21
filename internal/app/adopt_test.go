package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

func TestAdopt(t *testing.T) {
	f := &fake{status: map[string]claude.AuthInfo{}}
	a := addApp(t, map[string]*state.Login{"a1": {Name: "work", Kind: state.SignIn}}, "", f)

	cfg := t.TempDir()
	a.Env = append(a.Env, "CLAUDE_CONFIG_DIR="+cfg)
	src := filepath.Join(cfg, ".credentials.json")

	if _, err := a.Adopt(context.Background(), "stray"); err == nil {
		t.Error("adopt with no file: want an error")
	}

	if err := os.WriteFile(src, []byte(`{"claudeAiOauth":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Adopt(context.Background(), "work"); err == nil {
		t.Error("adopt into an existing name: want an error")
	}

	res, err := a.Adopt(context.Background(), "stray")
	if err != nil {
		t.Fatal(err)
	}
	if !res.BecameDefault {
		t.Error("there was no default; stray should be it")
	}

	id, _, _ := load(t, a).ByName("stray")
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Error("the file is still in ~/.claude: it must move, not be copied")
	}
	if _, err := os.Stat(filepath.Join(a.Store.Dir(id), ".credentials.json")); err != nil {
		t.Errorf("the file is not in the login: %v", err)
	}
}
