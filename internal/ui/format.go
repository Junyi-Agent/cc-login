package ui

import (
	"encoding/json"
	"io"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/app"
)

// Window is one quota window in list --json.
type Window struct {
	LeftPercent int        `json:"left_percent"`
	ResetsAt    *time.Time `json:"resets_at"`
}

// Login is one login in list --json. Times are RFC 3339; unknown values are
// null, and so is the quota when the API did not return it.
type Login struct {
	Name      string     `json:"name"`
	Default   bool       `json:"default"`
	Kind      string     `json:"kind"`   // sign-in, token, api-key
	Status    string     `json:"status"` // signed-in, expiring, expired, signed-out
	Account   *string    `json:"account"`
	Plan      *string    `json:"plan"`
	ExpiresAt *time.Time `json:"expires_at"`
	FiveHour  *Window    `json:"five_hour"`
	SevenDay  *Window    `json:"seven_day"`
}

// JSON writes the logins as an indented JSON array.
func JSON(w io.Writer, views []app.View) error {
	logins := make([]Login, 0, len(views))
	for _, v := range views {
		logins = append(logins, login(v))
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(logins)
}

func login(v app.View) Login {
	l := Login{
		Name: v.Name, Default: v.Default, Kind: string(v.Kind), Status: v.Status.String(),
		Account: strPtr(v.Account), Plan: strPtr(v.Plan), ExpiresAt: timePtr(v.ExpiresAt),
	}

	if v.Quota != nil {
		l.FiveHour = window(v.Quota.FiveHour)
		l.SevenDay = window(v.Quota.SevenDay)
	}
	return l
}

func window(w *anthropic.Window) *Window {
	left, ok := w.Left()
	if !ok {
		return nil
	}

	out := &Window{LeftPercent: left}
	if w.ResetsAt != nil {
		out.ResetsAt = timePtr(*w.ResetsAt)
	}
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	t = t.UTC()
	return &t
}
