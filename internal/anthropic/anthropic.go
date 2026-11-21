// Package anthropic calls the Anthropic API with a login's credentials.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client calls the API at BaseURL.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

// New is a client of https://api.anthropic.com, or of $CC_LOGIN_API_URL
// (for tests).
func New() *Client {
	base := os.Getenv("CC_LOGIN_API_URL")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	return &Client{HTTP: &http.Client{}, BaseURL: base}
}

// Window is one quota window of a plan.
type Window struct {
	Used     *float64   `json:"utilization"` // percent used; nil when unknown
	ResetsAt *time.Time `json:"resets_at"`   // nil when unknown
}

// Left is the percent left, rounded, 0 to 100; false when the plan has no
// such window or its use is unknown.
func (w *Window) Left() (int, bool) {
	if w == nil || w.Used == nil {
		return 0, false
	}
	return max(0, min(100, 100-int(math.Round(*w.Used)))), true
}

// Quota is the 5-hour and the weekly window, as claude's /usage shows them;
// nil for a window the plan does not have.
type Quota struct {
	FiveHour *Window `json:"five_hour"`
	SevenDay *Window `json:"seven_day"`
}

// Usage reads the plan quota with the OAuth access token of a sign-in, from
// the endpoint claude's /usage reads (api/oauth/usage, not documented).
func (c *Client) Usage(ctx context.Context, accessToken string) (Quota, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/oauth/usage", nil)
	if err != nil {
		return Quota{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Quota{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return Quota{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var q Quota
	err = json.NewDecoder(resp.Body).Decode(&q)
	return q, err
}

// Profile is what api/oauth/profile says about the account of a token.
type Profile struct {
	Email   string // "" when unknown
	Org     string // the organization's name; "" when unknown
	OrgType string // organization_type: claude_pro, claude_max, claude_team...
}

// Plan is the plan the organization type names: claude_team is team,
// claude_max is max; "" when unknown.
func (p Profile) Plan() string { return strings.TrimPrefix(p.OrgType, "claude_") }

// Profile reads the account and the organization of the OAuth token that
// has the user:profile scope (api/oauth/profile, not documented).
func (c *Client) Profile(ctx context.Context, accessToken string) (Profile, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/oauth/profile", nil)
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Profile{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return Profile{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var body struct {
		Account struct {
			Email string `json:"email"`
		} `json:"account"`
		Organization struct {
			Name string `json:"name"`
			Type string `json:"organization_type"`
		} `json:"organization"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Profile{}, err
	}

	return Profile{Email: body.Account.Email, Org: body.Organization.Name, OrgType: body.Organization.Type}, nil
}

// Verdict is what the API said about a key.
type Verdict int

// The verdicts.
const (
	Accepted Verdict = iota
	Rejected         // HTTP 401 or 403
	Unknown          // no answer, or another status: the network, a rate limit, an outage
)

func (v Verdict) String() string { return [...]string{"accepted", "rejected", "unknown"}[v] }

// verdict reads an HTTP status; 0 is no response.
func verdict(status int) Verdict {
	switch status {
	case http.StatusOK:
		return Accepted
	case http.StatusUnauthorized, http.StatusForbidden:
		return Rejected
	}
	return Unknown
}

// CheckKey asks the API whether it accepts API key key: a free request that
// lists the models. status is the HTTP status, 0 without a response.
func (c *Client) CheckKey(ctx context.Context, key string) (v Verdict, status int) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/models", nil)
	if err != nil {
		return Unknown, 0
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Unknown, 0
	}
	_ = resp.Body.Close()

	return verdict(resp.StatusCode), resp.StatusCode
}
