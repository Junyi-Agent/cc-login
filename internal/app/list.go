package app

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// View is what `list` shows about one login.
type View struct {
	Name    string
	Default bool
	Kind    state.Kind
	Account string // "" when unknown
	Plan    string // "" when unknown

	Status    Status
	ExpiresAt time.Time // zero when unknown or never

	Quota    *anthropic.Quota // the live quota; nil when not read
	QuotaErr error            // why reading the quota failed
}

// List probes every login: claude auth status and the quota, a few logins
// at a time. It ends early when ctx is cancelled.
func (a *App) List(ctx context.Context) ([]View, error) {
	st, err := a.Store.Load()
	if err != nil {
		return nil, err
	}

	ids := st.IDs()
	if len(ids) == 0 {
		return nil, nil
	}

	// Without claude every login would show signed out: say so instead.
	path, err := a.Claude.Path()
	if err != nil {
		return nil, err
	}
	a.Log.Debug("list", "root", a.Store.Root, "logins", len(ids), "claude", path, "parallel", a.Parallel)

	views := make([]View, len(ids))
	var g errgroup.Group
	g.SetLimit(a.Parallel)
	for i, id := range ids {
		g.Go(func() error {
			views[i] = a.probe(ctx, id, st.Logins[id], id == st.Default)
			return nil
		})
	}
	_ = g.Wait() // probe reports through the view, never by an error

	return views, ctx.Err()
}

// probe finds the status and the quota of one login.
func (a *App) probe(ctx context.Context, id string, l *state.Login, isDefault bool) (v View) {
	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()

	start := time.Now()
	defer func() {
		a.Log.Debug("probe", "login", l.Name, "id", id, "kind", l.Kind, "status", v.Status,
			"quota", v.Quota != nil, "quota_err", v.QuotaErr, "took", time.Since(start).Round(time.Millisecond))
	}()

	v = View{Name: l.Name, Default: isDefault, Kind: l.Kind, Account: l.Account, Plan: l.Plan, Status: SignedOut}
	if l.Kind == state.APIKey {
		v.Account = "API key"
		if l.KeyHint != "" {
			v.Account += " …" + l.KeyHint
		}
		v.Plan = "api"
	}

	env, err := a.env(id, l.Kind)
	if err != nil {
		return v // the key or token file is gone: claude cannot sign in
	}

	cs, err := a.Claude.AuthStatus(ctx, env)
	if err != nil {
		a.Log.Debug("claude auth status failed", "login", l.Name, "err", err)
	}
	if err == nil && cs.SignedIn {
		v.Status = SignedIn
	}

	// claude reports no expiry for a token login; state.json keeps one.
	v.ExpiresAt = cs.ExpiresAt
	if l.Kind == state.Token && l.TokenExpiresAt != nil {
		v.ExpiresAt = *l.TokenExpiresAt
	}

	now := a.Now()
	switch {
	case !v.ExpiresAt.IsZero() && !v.ExpiresAt.After(now):
		v.Status = Expired
	case v.Status == SignedIn && !v.ExpiresAt.IsZero() && v.ExpiresAt.Sub(now) < expiringWithin:
		v.Status = Expiring
	}

	// Only a sign-in's access token can read the quota.
	if v.Status.Usable() && l.Kind == state.SignIn {
		v.Quota, v.QuotaErr = a.quota(ctx, a.Store.Dir(id))
	}

	return v
}

// quota reads the live quota with the access token of the sign-in stored in
// dir; nil without one.
func (a *App) quota(ctx context.Context, dir string) (*anthropic.Quota, error) {
	tok, expires := a.AccessToken(ctx, dir)
	if tok == "" {
		return nil, nil
	}

	// 30 seconds to spare for the request.
	if expires.Before(a.Now().Add(30 * time.Second)) {
		return nil, ErrStaleToken
	}

	q, err := a.Usage(ctx, tok)
	if err != nil {
		return nil, fmt.Errorf("the quota request failed: %w", err)
	}
	return &q, nil
}
