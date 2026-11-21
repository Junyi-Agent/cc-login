// Package listing builds the table that `cc-login list` prints.
package listing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/claude"
	"github.com/Junyi-Agent/cc-login/internal/creds"
	"github.com/Junyi-Agent/cc-login/internal/quota"
	"github.com/Junyi-Agent/cc-login/internal/store"
)

// Login is what the list knows about one login.
type Login struct {
	Name     string
	Email    string // recorded account, "-" when unknown
	Plan     string // recorded plan, "-" when unknown
	Token    bool   // a token login (claude setup-token)
	Key      bool   // an API key login
	SignedIn bool   // claude auth status succeeded
	Expiry   int64  // when the sign-in or token expires; 0 when unknown

	Quota    quota.Quota
	QuotaOK  bool   // Quota holds a value, live or cached
	Cached   bool   // Quota came from the cache
	CachedAt int64  // when the cache entry was written; 0 when unknown
	QuotaWhy string // why the live quota failed: "", "expired" or "error"
}

// Prober gathers the facts; its fields are replaceable for tests.
type Prober struct {
	Store       *store.Store
	Now         time.Time
	Env         []string
	CachePath   string
	AuthStatus  func(ctx context.Context, env []string) ([]byte, bool)
	AccessToken func(ctx context.Context, dir string, now time.Time) (string, creds.Result)
	FetchQuota  func(ctx context.Context, tok string) (quota.Quota, error)
}

// NewProber returns a Prober that runs claude and reads the network.
func NewProber(s *store.Store) *Prober {
	client := &http.Client{}
	return &Prober{
		Store:       s,
		Now:         time.Now(),
		Env:         claude.Environ(),
		CachePath:   quota.CachePath(),
		AuthStatus:  claude.AuthStatus,
		AccessToken: creds.AccessToken,
		FetchQuota: func(ctx context.Context, tok string) (quota.Quota, error) {
			return quota.Fetch(ctx, client, tok)
		},
	}
}

// All gathers every login at once: each one runs claude auth status and a
// quota request, so they run in parallel.
func (p *Prober) All(ctx context.Context, names []string) []Login {
	out := make([]Login, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = p.One(ctx, name)
		}()
	}
	wg.Wait()
	return out
}

// recordFields reads [account, plan] from a record, as the zsh version's jq
// filter does: an API key login shows "API key …wxyz" and plan api.
func recordFields(rec map[string]any) (string, string) {
	apiKey := rec["authMethod"] == "api_key"
	str := func(v any) (string, bool) {
		if v == nil || v == false {
			return "", false
		}
		if s, ok := v.(string); ok {
			return s, true
		}
		return fmt.Sprint(v), true
	}
	email, ok := str(rec["email"])
	if !ok {
		email = "-"
		if apiKey {
			hint, hasHint := str(rec["apiKeyHint"])
			if !hasHint {
				hint = "?"
			}
			email = "API key " + hint
		}
	}
	plan, ok := str(rec["subscriptionType"])
	if !ok {
		plan = "-"
		if apiKey {
			plan = "api"
		}
	}
	return email, plan
}

// loginExpiry reads loginExpiresAt from claude auth status --json, or from a
// record, in seconds since the epoch; 0 when there is none.
func loginExpiry(b []byte) int64 {
	var v struct {
		LoginExpiresAt string `json:"loginExpiresAt"`
	}
	if json.Unmarshal(b, &v) != nil || v.LoginExpiresAt == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, v.LoginExpiresAt)
	if err != nil {
		return 0
	}
	return t.Unix()
}

// One gathers the facts of login name.
func (p *Prober) One(ctx context.Context, name string) Login {
	l := Login{Name: name, Email: "-", Plan: "-"}
	if rec, err := p.Store.Record(name); err == nil {
		l.Email, l.Plan = recordFields(rec)
	}
	dir, ok := p.Store.Dir(name)
	if !ok {
		return l
	}
	kind := store.DirKind(dir)
	l.Token = kind == store.KindToken
	l.Key = kind == store.KindKey

	out, signed := p.AuthStatus(ctx, claude.Env(p.Env, dir))
	l.SignedIn = signed
	// claude reports no expiry for a token: the token login's record keeps it.
	if l.Token {
		out = nil
		if b, err := readFile(p.Store.RecordPath(name)); err == nil {
			out = b
		}
	}
	l.Expiry = loginExpiry(out)

	now := p.Now.Unix()
	if !signed || (l.Expiry != 0 && l.Expiry <= now) {
		return l // signed out or expired: no quota
	}

	// The live quota needs a sign-in's access token. An API key login has
	// none. A token login's own token cannot read the quota; the access token
	// of a sign-in stored under it can, while valid.
	if kind != store.KindKey {
		tok, res := p.AccessToken(ctx, dir, p.Now)
		switch {
		case res == creds.OK:
			if q, err := p.FetchQuota(ctx, tok); err == nil {
				l.Quota, l.QuotaOK = q, true
				return l
			}
			l.QuotaWhy = "error"
		case res == creds.Expired && !l.Token:
			l.QuotaWhy = "expired"
		}
	}
	if q, at, err := quota.Cached(p.CachePath, filepath.Base(dir), p.Now); err == nil {
		l.Quota, l.QuotaOK, l.Cached, l.CachedAt = q, true, true, at
	}
	return l
}
