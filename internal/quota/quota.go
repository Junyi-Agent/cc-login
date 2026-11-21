// Package quota reads the plan quota that claude's /usage shows: live from
// the endpoint that claude reads it from (api/oauth/usage, not documented), or
// from the cache file that a status line command writes.
package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Window is one quota window. Used is nil when the plan has no such limit.
type Window struct {
	Used  *int  // percent used, rounded
	Reset int64 // when it resets, in seconds since the epoch; 0 when unknown
}

// Quota is the 5-hour and the weekly window.
type Quota struct {
	FiveHour Window
	SevenDay Window
}

// Any reports whether either window has a value.
func (q Quota) Any() bool { return q.FiveHour.Used != nil || q.SevenDay.Used != nil }

// UsageURL is the endpoint that claude's /usage reads.
var UsageURL = "https://api.anthropic.com/api/oauth/usage"

type usageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *string  `json:"resets_at"`
}

type usageResponse struct {
	FiveHour *usageWindow `json:"five_hour"`
	SevenDay *usageWindow `json:"seven_day"`
}

func roundPtr(f *float64) *int {
	if f == nil {
		return nil
	}
	n := int(math.Round(*f))
	return &n
}

func (w *usageWindow) window() (Window, error) {
	if w == nil {
		return Window{}, nil
	}
	out := Window{Used: roundPtr(w.Utilization)}
	if w.ResetsAt != nil {
		t, err := time.Parse(time.RFC3339Nano, *w.ResetsAt)
		if err != nil {
			return Window{}, err
		}
		out.Reset = t.Unix()
	}
	return out, nil
}

// Fetch reads the quota with the OAuth access token tok.
func Fetch(ctx context.Context, client *http.Client, tok string) (Quota, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, UsageURL, nil)
	if err != nil {
		return Quota{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	resp, err := client.Do(req)
	if err != nil {
		return Quota{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return Quota{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var u usageResponse
	if err = json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return Quota{}, err
	}
	var q Quota
	if q.FiveHour, err = u.FiveHour.window(); err != nil {
		return Quota{}, err
	}
	if q.SevenDay, err = u.SevenDay.window(); err != nil {
		return Quota{}, err
	}
	return q, nil
}

// CachePath is the quota cache: $CC_LOGIN_QUOTA_CACHE (the zsh shim passes the
// zstyle ':cc-login:' quota-cache there), else ~/.claude/quota-last.json.
func CachePath() string {
	home, _ := os.UserHomeDir()
	if p := os.Getenv("CC_LOGIN_QUOTA_CACHE"); p != "" {
		if strings.HasPrefix(p, "~/") {
			p = filepath.Join(home, p[2:])
		}
		return p
	}
	return filepath.Join(home, ".claude", "quota-last.json")
}

// CacheWindow is a window as the status line writes it (Claude Code's rate_limits).
type CacheWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"`
}

// CacheEntry is the entry "dir:ID" of the cache.
type CacheEntry struct {
	FiveHour *CacheWindow `json:"five_hour,omitempty"`
	SevenDay *CacheWindow `json:"seven_day,omitempty"`
	At       *float64     `json:"at,omitempty"`
}

func (w *CacheWindow) window(now int64) Window {
	if w == nil || w.UsedPercentage == nil {
		return Window{}
	}
	reset := now
	if w.ResetsAt != nil {
		reset = int64(*w.ResetsAt)
	}
	if reset < now {
		return Window{} // it has reset since
	}
	out := Window{Used: roundPtr(w.UsedPercentage)}
	if w.ResetsAt != nil {
		out.Reset = int64(*w.ResetsAt)
	}
	return out
}

// ErrNoCache means the cache holds nothing usable for the directory.
var ErrNoCache = errors.New("no cached quota")

// Cached reads the quota cached for the credentials directory whose ID is id,
// leaving out a window that has reset since. at is when it was written, or 0.
func Cached(path, id string, now time.Time) (q Quota, at int64, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Quota{}, 0, err
	}
	var cache map[string]json.RawMessage
	if err := json.Unmarshal(b, &cache); err != nil {
		return Quota{}, 0, err
	}
	raw, ok := cache["dir:"+id]
	if !ok {
		return Quota{}, 0, ErrNoCache
	}
	var e CacheEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return Quota{}, 0, err
	}
	n := now.Unix()
	q = Quota{FiveHour: e.FiveHour.window(n), SevenDay: e.SevenDay.window(n)}
	if !q.Any() {
		return Quota{}, 0, ErrNoCache
	}
	if e.At != nil {
		at = int64(*e.At)
	}
	return q, at, nil
}
