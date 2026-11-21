package listing

import (
	"encoding/json"
	"io"
	"time"
)

// Status is the STATUS column of login l and its tone: "signed in", "token",
// "signed out" or "expired"; tone ok, warn (expires within 7 days), bad
// (expired) or out (signed out).
func Status(l Login, now time.Time) (state, tone string) {
	if l.SignedIn {
		state, tone = "signed in", "ok"
		if l.Token {
			state = "token"
		}
	} else {
		state, tone = "signed out", "out"
	}
	if l.Expiry != 0 {
		left := l.Expiry - now.Unix()
		switch {
		case left <= 0:
			state, tone = "expired", "bad"
		case tone == "ok" && left < 7*86400:
			tone = "warn"
		}
	}
	return state, tone
}

// JSONWindow is one quota window in `list --json`.
type JSONWindow struct {
	LeftPercent int        `json:"left_percent"`
	ResetsAt    *time.Time `json:"resets_at"`
}

// JSONLogin is one login in `list --json`. Times are RFC 3339; null when unknown.
type JSONLogin struct {
	Name     string      `json:"name"`
	Default  bool        `json:"default"`
	Account  *string     `json:"account"`
	Plan     *string     `json:"plan"`
	Kind     string      `json:"kind"`   // "sign-in", "token" or "api-key"
	Status   string      `json:"status"` // as the STATUS column
	Expires  *time.Time  `json:"expires_at"`
	FiveHour *JSONWindow `json:"five_hour"`
	SevenDay *JSONWindow `json:"seven_day"`
	// QuotaSource is "live", "cache" or null; CachedAt is set for "cache".
	QuotaSource *string    `json:"quota_source"`
	CachedAt    *time.Time `json:"cached_at,omitempty"`
}

func timePtr(sec int64) *time.Time {
	if sec == 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

func strPtr(s string) *string {
	if s == "-" || s == "" {
		return nil
	}
	return &s
}

func jsonWindow(used *int, reset int64) *JSONWindow {
	if used == nil {
		return nil
	}
	left := 100 - *used
	if *used >= 100 {
		left = 0
	}
	return &JSONWindow{LeftPercent: left, ResetsAt: timePtr(reset)}
}

// ToJSON converts logins for `list --json`. The quota is left out for a login
// that is signed out or expired, as in the table.
func ToJSON(logins []Login, current string, now time.Time) []JSONLogin {
	out := make([]JSONLogin, 0, len(logins))
	for _, l := range logins {
		state, tone := Status(l, now)
		j := JSONLogin{
			Name: l.Name, Default: l.Name == current,
			Account: strPtr(l.Email), Plan: strPtr(l.Plan),
			Kind: "sign-in", Status: state, Expires: timePtr(l.Expiry),
		}
		switch {
		case l.Token:
			j.Kind = "token"
		case l.Key:
			j.Kind = "api-key"
		}
		if l.QuotaOK && tone != "out" && tone != "bad" {
			j.FiveHour = jsonWindow(l.Quota.FiveHour.Used, l.Quota.FiveHour.Reset)
			j.SevenDay = jsonWindow(l.Quota.SevenDay.Used, l.Quota.SevenDay.Reset)
			src := "live"
			if l.Cached {
				src = "cache"
				j.CachedAt = timePtr(l.CachedAt)
			}
			j.QuotaSource = &src
		}
		out = append(out, j)
	}
	return out
}

// RenderJSON writes `list --json`: one JSON array, indented.
func RenderJSON(w io.Writer, logins []Login, current string, now time.Time) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(ToJSON(logins, current, now))
}
