package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Level is how bad a finding of Doctor is.
type Level int

// The levels, from the best.
const (
	OK   Level = iota
	Warn       // works, but not as it should
	Fail       // something does not work
)

// Finding is one check of Doctor: what it found and, unless it is OK, what
// to do about it.
type Finding struct {
	Level Level
	What  string
	Fix   string // "" for OK
}

// Doctor checks the setup, offline: claude and the claude link, the shell's
// own keys, state.json, and each login's files and expiry. It never reads a
// key or a token, only whether the files are there and who can read them.
func (a *App) Doctor() []Finding {
	var fs []Finding
	fs = append(fs, a.checkClaude()...)
	fs = append(fs, a.checkShell()...)
	fs = append(fs, a.checkLogins()...)

	if stray := a.StrayCredentials(); stray != "" {
		fs = append(fs, Finding{Warn, stray + " holds a sign-in that a claude run without cc-login wrote", "cc-login adopt NAME"})
	}
	return fs
}

// Problems counts the findings of level Fail.
func Problems(fs []Finding) int {
	n := 0
	for _, f := range fs {
		if f.Level == Fail {
			n++
		}
	}
	return n
}

// checkClaude checks the claude cc-login runs, and that a plain claude goes
// through cc-login (the claude link, ADR 0007).
func (a *App) checkClaude() []Finding {
	s := a.Setup()
	var fs []Finding

	if s.ClaudeErr != nil {
		fs = append(fs, Finding{Fail, "claude: " + s.ClaudeErr.Error(), "install Claude Code, or set CC_LOGIN_CLAUDE to its path"})
	} else {
		fs = append(fs, Finding{OK, "claude: " + s.Claude, ""})
	}

	switch {
	case s.Linked:
		fs = append(fs, Finding{OK, "a plain claude goes through cc-login: " + s.InPath, ""})
	case s.InPath == "":
		fs = append(fs, Finding{Fail, "PATH has no claude, so no claude link either", "link cc-login as claude, first in PATH (README: Getting Started)"})
	default:
		fs = append(fs, Finding{Fail, "a plain claude runs " + s.InPath + " without cc-login; the default login does not apply",
			"link cc-login as claude, and put its directory before " + filepath.Dir(s.InPath) + " in PATH (README: Getting Started)"})
	}
	return fs
}

// shellVars are the variables that make a claude without the link use a
// key, token or cloud provider of the shell; cc-login drops them from the
// claude it starts (creds.Env).
var shellVars = []string{
	"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY",
	"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN",
}

// checkShell checks the shell's environment for keys, tokens and cloud
// providers of its own.
func (a *App) checkShell() []Finding {
	var fs []Finding

	for _, v := range shellVars {
		if lookup(a.Env, v) != "" {
			fs = append(fs, Finding{Warn, v + " is set in the shell: a claude without the link uses it, whatever the default login is",
				"unset it; cc-login does not pass it on to its claude"})
		}
	}
	return fs
}

// checkLogins checks state.json, the default login and every login.
func (a *App) checkLogins() []Finding {
	st, err := a.Store.Load()
	if err != nil {
		return []Finding{{Fail, "state.json: " + err.Error(), "fix or move the file; cc-login never rewrites a state.json it cannot read"}}
	}

	var fs []Finding
	switch {
	case len(st.Logins) == 0:
		return []Finding{{Warn, "no logins yet", "cc-login add NAME"}}
	case st.Logins[st.Default] == nil:
		fs = append(fs, Finding{Fail, "no default login: claude through cc-login does not start", "cc-login use NAME"})
	default:
		fs = append(fs, Finding{OK, "default login: " + st.Logins[st.Default].Name, ""})
	}

	for _, id := range st.IDs() {
		fs = append(fs, a.checkLogin(id, st.Logins[id])...)
	}
	return fs
}

// checkLogin checks one login: its directory, its key or token file, and
// when the token expires.
func (a *App) checkLogin(id string, l *state.Login) []Finding {
	dir := a.Store.Dir(id)
	var fs []Finding

	fi, err := os.Stat(dir)
	switch {
	case err != nil:
		return []Finding{{Fail, "login " + l.Name + ": its credentials directory is gone", "cc-login remove " + l.Name + ", then add it again"}}
	case fi.Mode().Perm()&0o077 != 0:
		fs = append(fs, Finding{Warn, fmt.Sprintf("login %s: its directory is %04o; others can read it", l.Name, fi.Mode().Perm()), "chmod 700 " + dir})
	}

	if l.Kind != state.SignIn {
		fs = append(fs, a.checkSecretFile(dir, l)...)
	}
	if l.Kind == state.Token {
		fs = append(fs, a.checkToken(l)...)
	}

	if len(fs) == 0 || !slices.ContainsFunc(fs, func(f Finding) bool { return f.Level != OK }) {
		fs = append(fs, Finding{OK, "login " + l.Name + ": ok", ""})
	}
	return fs
}

// checkSecretFile checks that the key or token file of login l is there,
// and that only its owner can read it.
func (a *App) checkSecretFile(dir string, l *state.Login) []Finding {
	name := creds.APIKeyFile
	if l.Kind == state.Token {
		name = creds.TokenFile
	}
	path := filepath.Join(dir, name)

	fi, err := os.Stat(path)
	switch {
	case err != nil:
		return []Finding{{Fail, "login " + l.Name + ": " + name + " is gone", "cc-login remove " + l.Name + ", then add it again"}}
	case fi.Mode().Perm()&0o077 != 0:
		return []Finding{{Warn, fmt.Sprintf("login %s: %s is %04o; others can read it", l.Name, name, fi.Mode().Perm()), "chmod 600 " + path}}
	}
	return nil
}

// checkToken checks when the token of login l expires, and whether it can
// read its profile and quota (ADR 0017, 0018).
func (a *App) checkToken(l *state.Login) []Finding {
	var fs []Finding

	if exp := l.TokenExpiresAt; exp != nil {
		switch left := exp.Sub(a.Now()); {
		case left <= 0:
			fs = append(fs, Finding{Fail, "login " + l.Name + ": its token has expired", "cc-login renew " + l.Name})
		case left < expiringWithin:
			fs = append(fs, Finding{Warn, "login " + l.Name + ": its token expires within a week", "cc-login renew " + l.Name})
		}
	}

	if !slices.Contains(l.Scopes, oauthProfileScope) {
		fs = append(fs, Finding{Warn, "login " + l.Name + ": its token cannot read the account, organization and quota (no user:profile)", "cc-login renew " + l.Name})
	}
	return fs
}
