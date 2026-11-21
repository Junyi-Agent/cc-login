package app

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/quota"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Status is whether a login can be used.
type Status int

// The statuses, from the best.
const (
	SignedIn  Status = iota
	Expiring         // signed in, and the sign-in or token expires within a week
	Expired          // the sign-in or token has expired
	SignedOut        // claude auth status fails
)

// expiringWithin is how soon an expiry makes a login Expiring.
const expiringWithin = 7 * 24 * time.Hour

func (s Status) String() string {
	return [...]string{"signed-in", "expiring", "expired", "signed-out"}[s]
}

// Usable reports whether claude can use the login: signed in, expiring or not.
func (s Status) Usable() bool { return s == SignedIn || s == Expiring }

// QuotaSource is where View.Quota comes from.
type QuotaSource int

// The quota sources.
const (
	QuotaNone  QuotaSource = iota
	QuotaLive              // the API, now
	QuotaCache             // the cache, as of View.CachedAt
)

// QuotaGap is why a usable login has no live quota.
type QuotaGap int

// The reasons.
const (
	GapNone    QuotaGap = iota
	GapStale            // the sign-in's access token has expired; a claude run refreshes it
	GapError            // the request failed
	GapToken            // a setup-token token cannot read the quota
	GapNoLogin          // an API key login has no plan quota
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

	Quota       quota.Quota
	QuotaSource QuotaSource
	CachedAt    time.Time // QuotaCache: when the cache entry was written; zero when unknown
	QuotaGap    QuotaGap
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

	path, err := a.FindClaude()
	if err != nil {
		return nil, err
	}

	views := make([]View, len(ids))
	var g errgroup.Group
	g.SetLimit(a.Parallel)
	for i, id := range ids {
		g.Go(func() error {
			views[i] = a.probe(ctx, path, id, st.Logins[id], id == st.Default)
			return nil
		})
	}
	_ = g.Wait() // probe reports through the view, never by an error

	return views, ctx.Err()
}

// probe finds the status and the quota of one login.
func (a *App) probe(ctx context.Context, path, id string, l *state.Login, isDefault bool) View {
	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()

	v := View{Name: l.Name, Default: isDefault, Kind: l.Kind, Account: l.Account, Plan: l.Plan, Status: SignedOut}
	if l.Kind == state.APIKey {
		v.Account = "API key"
		if l.KeyHint != "" {
			v.Account += " …" + l.KeyHint
		}
		v.Plan = "api"
	}

	dir := a.Store.Dir(id)
	env, err := claude.Env(a.Env, dir, l.Kind)
	if err != nil {
		return v // the key or token file is gone: claude cannot sign in
	}

	cs, err := a.AuthStatus(ctx, path, env)
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

	if v.Status.Usable() {
		a.quota(ctx, &v, dir, id)
	}

	return v
}

// quota reads the live quota with the access token of the sign-in stored in
// dir; without one, the cached quota.
func (a *App) quota(ctx context.Context, v *View, dir, id string) {
	if v.Kind == state.APIKey {
		v.QuotaGap = GapNoLogin
	} else {
		// A token login's own token cannot read the quota; the access token
		// of a sign-in stored under it can, while valid.
		tok, res := a.AccessToken(ctx, dir, a.Now())
		switch {
		case res == creds.OK:
			q, err := a.Usage(ctx, tok)
			if err == nil {
				v.Quota, v.QuotaSource = q, QuotaLive
				return
			}
			v.QuotaGap = GapError

		case v.Kind == state.Token:
			v.QuotaGap = GapToken

		case res == creds.Expired:
			v.QuotaGap = GapStale
		}
	}

	if q, at, err := quota.Cached(a.QuotaCache, id, a.Now()); err == nil {
		v.Quota, v.QuotaSource, v.CachedAt = q, QuotaCache, at
	}
}
