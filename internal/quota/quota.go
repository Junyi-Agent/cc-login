// Package quota is the plan quota that claude's /usage shows, and the cache
// file that keeps the last quota seen for each credentials directory.
package quota

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Window is one quota window. Used is nil when the plan has no such limit.
type Window struct {
	Used     *int      // percent used, rounded
	ResetsAt time.Time // zero when unknown
}

// Left is the percent left, 0 to 100.
func (w Window) Left() int {
	if w.Used == nil {
		return 100
	}
	return max(0, min(100, 100-*w.Used))
}

// Quota is the 5-hour and the weekly window.
type Quota struct {
	FiveHour Window
	SevenDay Window
}

// Any reports whether either window has a value.
func (q Quota) Any() bool { return q.FiveHour.Used != nil || q.SevenDay.Used != nil }

// Round is f rounded to a whole percent; nil stays nil.
func Round(f *float64) *int {
	if f == nil {
		return nil
	}

	n := int(math.Round(*f))
	return &n
}

// CachePath is the quota cache: $CC_LOGIN_QUOTA_CACHE, else
// ~/.claude/quota-last.json.
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

// cacheWindow is a window as the cache keeps it (Claude Code's rate_limits).
type cacheWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"` // seconds since the epoch
}

// cacheEntry is the entry "dir:ID" of the cache.
type cacheEntry struct {
	FiveHour *cacheWindow `json:"five_hour,omitempty"`
	SevenDay *cacheWindow `json:"seven_day,omitempty"`
	At       *float64     `json:"at,omitempty"` // when it was written
}

// window is w as of now; a window that has reset since is empty.
func (w *cacheWindow) window(now time.Time) Window {
	if w == nil || w.UsedPercentage == nil {
		return Window{}
	}

	out := Window{Used: Round(w.UsedPercentage)}
	if w.ResetsAt != nil {
		out.ResetsAt = unix(*w.ResetsAt)
		if out.ResetsAt.Before(now) {
			return Window{}
		}
	}

	return out
}

func unix(sec float64) time.Time { return time.Unix(int64(sec), 0).UTC() }

// ErrNoCache means the cache holds nothing usable for the directory.
var ErrNoCache = errors.New("no cached quota")

// Cached reads the quota cached for the credentials directory with that ID,
// without the windows that have reset since. at is when it was written;
// zero when the cache does not say.
func Cached(path, id string, now time.Time) (q Quota, at time.Time, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Quota{}, time.Time{}, err
	}

	var cache map[string]json.RawMessage
	if err := json.Unmarshal(b, &cache); err != nil {
		return Quota{}, time.Time{}, err
	}

	raw, ok := cache["dir:"+id]
	if !ok {
		return Quota{}, time.Time{}, ErrNoCache
	}

	var e cacheEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return Quota{}, time.Time{}, err
	}

	q = Quota{FiveHour: e.FiveHour.window(now), SevenDay: e.SevenDay.window(now)}
	if !q.Any() {
		return Quota{}, time.Time{}, ErrNoCache
	}

	if e.At != nil {
		at = unix(*e.At)
	}

	return q, at, nil
}
