// Package claude finds the claude CLI and runs its auth commands
// (claude auth status, login, logout) in the environment it is given;
// package creds makes that environment.
package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/term"
)

// CLI is the claude that Find finds, found again at each call: Find only
// looks at PATH.
type CLI struct{}

// Path is the claude executable (Find).
func (CLI) Path() (string, error) { return Find() }

// AuthStatus runs claude auth status (AuthStatus) with env.
func (CLI) AuthStatus(ctx context.Context, env []string) (AuthInfo, error) {
	path, err := Find()
	if err != nil {
		return AuthInfo{}, err
	}
	return AuthStatus(ctx, path, env)
}

// AuthLogin runs claude auth login (AuthLogin) with env and args.
func (CLI) AuthLogin(ctx context.Context, env, args []string) error {
	path, err := Find()
	if err != nil {
		return err
	}
	return AuthLogin(ctx, path, env, args)
}

// AuthLogout runs claude auth logout (AuthLogout) with env.
func (CLI) AuthLogout(ctx context.Context, env []string) error {
	path, err := Find()
	if err != nil {
		return err
	}
	return AuthLogout(ctx, path, env)
}

// ErrNotFound means there is no claude to run.
var ErrNotFound = errors.New("claude is not installed, or not in PATH")

// Find is the claude executable: $CC_LOGIN_CLAUDE, else the first claude in
// PATH that is not this program (cc-login can be installed as claude, ADR
// 0007).
func Find() (string, error) {
	if p := os.Getenv("CC_LOGIN_CLAUDE"); p != "" {
		return p, nil
	}

	self, _ := os.Executable()
	selfInfo, _ := os.Stat(self)

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		p := filepath.Join(dir, "claude")

		// PATH is the user's own setting; Find only looks at what it lists.
		fi, err := os.Stat(p) //nolint:gosec // G703: see above
		if err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
			continue
		}
		if selfInfo != nil && os.SameFile(fi, selfInfo) {
			continue
		}

		return p, nil
	}

	return "", ErrNotFound
}

// AuthInfo is what `claude auth status --json` says about a login.
type AuthInfo struct {
	SignedIn  bool
	ExpiresAt time.Time // zero when claude does not say
	Email     string
	OrgID     string
	OrgName   string
	Plan      string // subscriptionType
}

type statusJSON struct {
	LoginExpiresAt   string `json:"loginExpiresAt"`
	Email            string `json:"email"`
	OrgID            string `json:"orgId"`
	OrgName          string `json:"orgName"`
	SubscriptionType string `json:"subscriptionType"`
}

// AuthStatus runs `claude auth status --json` (the claude at path) with env.
// claude exits 0 when the login is signed in. An error means claude did not
// run at all.
func AuthStatus(ctx context.Context, path string, env []string) (AuthInfo, error) {
	cmd := exec.CommandContext(ctx, path, "auth", "status", "--json")
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return AuthInfo{}, fmt.Errorf("run %s: %w", path, err)
	}

	st := AuthInfo{SignedIn: err == nil}

	var j statusJSON
	if json.Unmarshal(out.Bytes(), &j) == nil {
		st.Email, st.OrgID, st.OrgName, st.Plan = j.Email, j.OrgID, j.OrgName, j.SubscriptionType
		if t, err := time.Parse(time.RFC3339Nano, j.LoginExpiresAt); err == nil {
			st.ExpiresAt = t
		}
	}

	return st, nil
}

// AuthLogout runs `claude auth logout` with env: it revokes the stored sign-in
// at Anthropic and deletes it. The error carries what claude printed.
func AuthLogout(ctx context.Context, path string, env []string) error {
	cmd := exec.CommandContext(ctx, path, "auth", "logout")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

// ErrStopped means the user stopped claude auth login (Ctrl-C).
var ErrStopped = errors.New("stopped")

// AuthLogin runs `claude auth login ARGS` with env, on this terminal. args are
// options such as --email, never secrets.
//
// When stdin is a terminal, claude's stdin is a pipe and cc-login passes it
// each line typed or pasted: claude reading a pasted code from a raw
// terminal can split it and reject it as an invalid code. A wrong code can
// be pasted again; finishing in the browser ends claude. Otherwise claude
// reads cc-login's stdin.
func AuthLogin(ctx context.Context, path string, env, args []string) error {
	cmd := exec.CommandContext(ctx, path, append([]string{"auth", "login"}, args...)...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		cmd.Stdin = os.Stdin
		return stopped(ctx, cmd.Run())
	}

	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdin = r

	if err = cmd.Start(); err != nil {
		_ = r.Close()
		_ = w.Close()
		return err
	}
	_ = r.Close() // claude has its end

	// Relay lines until claude exits. The goroutine may stay blocked in a
	// read of the terminal after that; cc-login exits soon after.
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if _, errW := fmt.Fprintln(w, sc.Text()); errW != nil {
				return
			}
		}
	}()

	err = cmd.Wait()
	_ = w.Close()
	return stopped(ctx, err)
}

// stopped is err, or ErrStopped when ctx was cancelled (Ctrl-C): claude
// exits 0 when it is stopped.
func stopped(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ErrStopped
	}
	return err
}
