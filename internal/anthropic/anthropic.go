// Package anthropic calls the Anthropic API with a login's credentials.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/quota"
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

type usageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *string  `json:"resets_at"`
}

type usageResponse struct {
	FiveHour *usageWindow `json:"five_hour"`
	SevenDay *usageWindow `json:"seven_day"`
}

func (w *usageWindow) window() (quota.Window, error) {
	if w == nil {
		return quota.Window{}, nil
	}

	out := quota.Window{Used: quota.Round(w.Utilization)}
	if w.ResetsAt != nil {
		t, err := time.Parse(time.RFC3339Nano, *w.ResetsAt)
		if err != nil {
			return quota.Window{}, err
		}
		out.ResetsAt = t.UTC()
	}

	return out, nil
}

// Usage reads the plan quota with the OAuth access token of a sign-in, from
// the endpoint claude's /usage reads (api/oauth/usage, not documented).
func (c *Client) Usage(ctx context.Context, accessToken string) (quota.Quota, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/oauth/usage", nil)
	if err != nil {
		return quota.Quota{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return quota.Quota{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return quota.Quota{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var u usageResponse
	if err = json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return quota.Quota{}, err
	}

	var q quota.Quota
	if q.FiveHour, err = u.FiveHour.window(); err != nil {
		return quota.Quota{}, err
	}
	if q.SevenDay, err = u.SevenDay.window(); err != nil {
		return quota.Quota{}, err
	}

	return q, nil
}

// Verdict is what the API said about a key or a token.
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

// CheckToken asks the API whether it accepts token tok from claude
// setup-token: a token count, which is free and uses no quota. org is the
// organization the token belongs to (the anthropic-organization-id header).
// A response with that header proves the token authenticated, whatever its
// status (a retired model gives 404).
func (c *Client) CheckToken(ctx context.Context, tok string) (v Verdict, org string, status int) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	body := strings.NewReader(`{"model":"claude-haiku-5-5","messages":[{"role":"user","content":"hi"}]}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages/count_tokens", body)
	if err != nil {
		return Unknown, "", 0
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Unknown, "", 0
	}
	_ = resp.Body.Close()

	org = strings.ReplaceAll(resp.Header.Get("anthropic-organization-id"), " ", "")
	v = verdict(resp.StatusCode)
	if v == Unknown && org != "" {
		v = Accepted
	}
	return v, org, resp.StatusCode
}
