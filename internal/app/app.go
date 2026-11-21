// Package app is what cc-login does, one function per command. It writes
// nothing to stdout and knows nothing of the command line: cmd/cc-login and
// the TUI call it (ADR 0009). The outside world (claude, the Keychain, the
// API, the clock) comes in through the fields, so tests can replace it.
package app

import (
	"context"
	"os"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/quota"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// App holds the store and the outside world.
type App struct {
	Store *state.Store
	Env   []string // the environment claude starts from
	Now   func() time.Time

	FindClaude  func() (string, error)
	AuthStatus  func(ctx context.Context, path string, env []string) (claude.Status, error)
	Logout      func(ctx context.Context, path string, env []string) error
	SignIn      func(ctx context.Context, path string, env, args []string) error
	CheckKey    func(ctx context.Context, key string) (anthropic.Verdict, int)
	CheckToken  func(ctx context.Context, tok string) (anthropic.Verdict, string, int)
	AccessToken func(ctx context.Context, dir string, now time.Time) (string, creds.Result)
	Usage       func(ctx context.Context, accessToken string) (quota.Quota, error)
	QuotaCache  string

	Parallel int           // logins probed at once
	Timeout  time.Duration // for probing one login
}

// New is the App of this machine.
func New() *App {
	api := anthropic.New()
	return &App{
		Store:       state.New(),
		Env:         os.Environ(),
		Now:         time.Now,
		FindClaude:  claude.Find,
		AuthStatus:  claude.AuthStatus,
		Logout:      claude.Logout,
		SignIn:      claude.SignIn,
		CheckKey:    api.CheckKey,
		CheckToken:  api.CheckToken,
		AccessToken: creds.AccessToken,
		Usage:       api.Usage,
		QuotaCache:  quota.CachePath(),
		Parallel:    8,
		Timeout:     15 * time.Second,
	}
}

// Names are the names of the logins, sorted; none when state.json cannot
// be read.
func (a *App) Names() []string {
	st, err := a.Store.Load()
	if err != nil {
		return nil
	}
	return st.Names()
}

// Migrate converts the zsh version's data layout into state.json (ADR 0008).
func (a *App) Migrate() (state.Migration, error) {
	return a.Store.Migrate(a.Now())
}
