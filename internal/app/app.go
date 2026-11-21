// Package app is what cc-login does, one function per command, one file
// per command. It writes nothing to stdout and knows nothing of the command
// line: cmd/cc-login calls it (ADR 0012). The outside world (claude, the
// Keychain, the API, the clock) comes in through the fields, so tests can
// replace it.
package app

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/oauth"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Claude runs the claude CLI in the environment it is given.
// claude.CLI is the real one.
type Claude interface {
	Path() (string, error) // the claude executable
	AuthStatus(ctx context.Context, env []string) (claude.AuthInfo, error)
	AuthLogin(ctx context.Context, env, args []string) error
	AuthLogout(ctx context.Context, env []string) error
	InPath() (string, bool) // what a plain claude runs, and whether it is cc-login
}

// Prompter asks the user. On a terminal Secret shows what is typed as * and
// Open opens a link in the browser; without one, Secret reads a line of
// stdin and Open does nothing.
type Prompter interface {
	Secret(title, help string, valid func(string) error) (string, error)
	Note(msg string)  // a line for the user, on stderr
	Open(link string) // in the browser, when it can
}

// App holds the store and the outside world.
type App struct {
	Store  *state.Store
	Env    []string // the environment claude starts from
	Now    func() time.Time
	Claude Claude

	CheckKey    func(ctx context.Context, key string) (anthropic.Verdict, int)
	Exchange    func(ctx context.Context, f oauth.Flow, code string) (oauth.Token, error)
	AccessToken func(ctx context.Context, dir string) (string, time.Time)
	Usage       func(ctx context.Context, accessToken string) (anthropic.Quota, error)

	Parallel int           // logins probed at once
	Timeout  time.Duration // for probing one login

	// Log is the debug log (cc-login --verbose). It records what runs and
	// what came back, never an environment, a key or a token.
	Log *slog.Logger
}

// New is the App of this machine.
func New() *App {
	api := anthropic.New()
	return &App{
		Store:       state.New(),
		Env:         os.Environ(),
		Now:         time.Now,
		Claude:      claude.CLI{},
		CheckKey:    api.CheckKey,
		Exchange:    oauth.New().Exchange,
		AccessToken: creds.AccessToken,
		Usage:       api.Usage,
		Parallel:    8,
		Timeout:     15 * time.Second,
		Log:         slog.New(slog.DiscardHandler),
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

// env is the environment of a claude run with login l, whose ID is id: its
// directory, and its key or token (creds.Env).
func (a *App) env(id string, l *state.Login) ([]string, error) {
	return creds.Env(a.Env, a.Store.Dir(id), l)
}

// storedEnv is the environment in which claude sees only the sign-in stored
// for the login with that ID, not the login's key or token.
func (a *App) storedEnv(id string) []string {
	return creds.StoredEnv(a.Env, a.Store.Dir(id))
}

// lookup is the value of variable key in env.
func lookup(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if len(env[i]) > len(key) && env[i][len(key)] == '=' && env[i][:len(key)] == key {
			return env[i][len(key)+1:]
		}
	}
	return ""
}
