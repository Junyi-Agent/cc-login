package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/app"
	"github.com/Junyi-Agent/cc-login/internal/quota"
)

// Window is one quota window in the machine-readable forms.
type Window struct {
	LeftPercent int        `json:"left_percent"`
	ResetsAt    *time.Time `json:"resets_at"`
}

// Login is one login in the machine-readable forms (list --format). Times
// are RFC 3339; unknown values are null.
type Login struct {
	Name        string     `json:"name"`
	Default     bool       `json:"default"`
	Kind        string     `json:"kind"`   // sign-in, token, api-key
	Status      string     `json:"status"` // signed-in, expiring, expired, signed-out
	Account     *string    `json:"account"`
	Plan        *string    `json:"plan"`
	ExpiresAt   *time.Time `json:"expires_at"`
	FiveHour    *Window    `json:"five_hour"`
	SevenDay    *Window    `json:"seven_day"`
	QuotaSource *string    `json:"quota_source"` // live, cache
	CachedAt    *time.Time `json:"cached_at,omitempty"`
}

// Logins converts the views for the machine-readable forms.
func Logins(views []app.View) []Login {
	out := make([]Login, 0, len(views))
	for _, v := range views {
		out = append(out, login(v))
	}
	return out
}

func login(v app.View) Login {
	l := Login{
		Name: v.Name, Default: v.Default, Kind: string(v.Kind), Status: v.Status.String(),
		Account: strPtr(v.Account), Plan: strPtr(v.Plan), ExpiresAt: timePtr(v.ExpiresAt),
	}

	switch v.QuotaSource {
	case app.QuotaLive:
		l.QuotaSource = strPtr("live")
	case app.QuotaCache:
		l.QuotaSource = strPtr("cache")
		l.CachedAt = timePtr(v.CachedAt)
	case app.QuotaNone:
		return l
	}

	l.FiveHour = window(v.Quota.FiveHour)
	l.SevenDay = window(v.Quota.SevenDay)
	return l
}

func window(w quota.Window) *Window {
	if w.Used == nil {
		return nil
	}
	return &Window{LeftPercent: w.Left(), ResetsAt: timePtr(w.ResetsAt)}
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

// Format writes the logins as format: "json" (an indented array), "tsv"
// (one line per login: name, default, kind, status, account, plan,
// expires_at, 5-hour and weekly percent left; empty when unknown), or a Go
// template applied to each Login.
func Format(w io.Writer, views []app.View, format string) error {
	logins := Logins(views)

	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(logins)

	case "tsv":
		for _, l := range logins {
			if _, err := fmt.Fprintln(w, strings.Join(tsv(l), "\t")); err != nil {
				return err
			}
		}
		return nil
	}

	tmpl, err := template.New("format").Parse(format)
	if err != nil {
		return fmt.Errorf("--format is json, tsv or a Go template: %w", err)
	}
	for _, l := range logins {
		if err := tmpl.Execute(w, l); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

// tsv are the fields of l for the tsv format.
func tsv(l Login) []string {
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	left := func(w *Window) string {
		if w == nil {
			return ""
		}
		return strconv.Itoa(w.LeftPercent)
	}
	expires := ""
	if l.ExpiresAt != nil {
		expires = l.ExpiresAt.Format(time.RFC3339)
	}

	return []string{
		l.Name, strconv.FormatBool(l.Default), l.Kind, l.Status, str(l.Account), str(l.Plan),
		expires, left(l.FiveHour), left(l.SevenDay),
	}
}
