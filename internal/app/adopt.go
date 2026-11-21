package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// StrayCredentials is the sign-in a claude run without cc-login wrote to
// ~/.claude (or $CLAUDE_CONFIG_DIR); "" when there is none.
func (a *App) StrayCredentials() string {
	cfg := lookup(a.Env, "CLAUDE_CONFIG_DIR")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".claude")
	}

	path := filepath.Join(cfg, ".credentials.json")
	if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() { //nolint:gosec // G703: the user's own CLAUDE_CONFIG_DIR, read as Claude Code reads it
		return ""
	}
	return path
}

// Adopt moves the stray sign-in (StrayCredentials) into a new login name.
// It moves the file, never copies it: two copies of one sign-in invalidate
// each other at the first token refresh. claude sessions that read it from
// ~/.claude lose their credentials.
func (a *App) Adopt(ctx context.Context, name string) (Saved, error) {
	src := a.StrayCredentials()
	if src == "" {
		return Saved{}, errors.New("there is no ~/.claude/.credentials.json to adopt")
	}

	if err := a.checkNewName(name); err != nil {
		return Saved{}, err
	}

	id, err := a.Store.NewDir()
	if err != nil {
		return Saved{}, err
	}

	dir := a.Store.Dir(id)
	if err = os.Rename(src, filepath.Join(dir, ".credentials.json")); err != nil {
		_ = os.RemoveAll(dir)
		return Saved{}, fmt.Errorf("move %s: %w (it stays where it is)", src, err)
	}

	l := &state.Login{Name: name, Kind: state.SignIn, CreatedAt: a.Now().UTC()}
	if cs, errStatus := a.Claude.AuthStatus(ctx, a.storedEnv(id)); errStatus == nil {
		l.Account, l.Plan = cs.Email, cs.Plan
	}

	return a.addLogin(id, l)
}
