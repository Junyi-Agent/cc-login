// Package anthropic calls the Anthropic API with a login's credentials.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
