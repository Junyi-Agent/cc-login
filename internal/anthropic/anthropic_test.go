package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// serve is a client of a test server that answers with h.
func serve(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), BaseURL: srv.URL}
}

func TestUsage(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/usage" || r.Header.Get("Authorization") != "Bearer good" ||
			r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":26.5,"resets_at":"2026-11-21T05:00:00.123456+00:00"},
			"seven_day":{"utilization":76.4,"resets_at":"2026-11-25T00:00:00Z"},"seven_day_opus":null}`))
	})

	q, err := c.Usage(context.Background(), "good")
	if err != nil {
		t.Fatal(err)
	}

	reset := time.Date(2026, 11, 21, 5, 0, 0, 123_456_000, time.UTC)
	if left, _ := q.FiveHour.Left(); left != 73 || !q.FiveHour.ResetsAt.Equal(reset) {
		t.Errorf("five_hour = %d%% left, resets %v", left, q.FiveHour.ResetsAt)
	}
	if left, _ := q.SevenDay.Left(); left != 24 {
		t.Errorf("seven_day = %d%% left", left)
	}

	if _, err := c.Usage(context.Background(), "bad"); err == nil {
		t.Error("401: want an error")
	}
}

// A plan without a window has a null one; a window can lack its reset time.
func TestUsageNoLimit(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"five_hour":null,"seven_day":{"utilization":3,"resets_at":null}}`))
	})

	q, err := c.Usage(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := q.FiveHour.Left(); ok {
		t.Error("five_hour is null: want no percent")
	}
	if left, ok := q.SevenDay.Left(); !ok || left != 97 || q.SevenDay.ResetsAt != nil {
		t.Errorf("seven_day = %d%% left, resets %v", left, q.SevenDay.ResetsAt)
	}
}

func TestUsageBadJSON(t *testing.T) {
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":1,"resets_at":"soon"}}`))
	})

	if _, err := c.Usage(context.Background(), "x"); err == nil {
		t.Error("a bad time: want an error")
	}
}

func TestWindowLeft(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		used *float64
		want int
	}{
		{f(0), 100},
		{f(26.5), 73},
		{f(130), 0},
		{f(-5), 100},
	}

	for _, tc := range cases {
		if got, ok := (&Window{Used: tc.used}).Left(); !ok || got != tc.want {
			t.Errorf("Left with %v used = %d, %v; want %d", *tc.used, got, ok, tc.want)
		}
	}
}

func TestNewBaseURL(t *testing.T) {
	t.Setenv("CC_LOGIN_API_URL", "")
	if got := New().BaseURL; got != "https://api.anthropic.com" {
		t.Errorf("BaseURL = %q", got)
	}

	t.Setenv("CC_LOGIN_API_URL", "http://127.0.0.1:9")
	if got := New().BaseURL; got != "http://127.0.0.1:9" {
		t.Errorf("BaseURL = %q", got)
	}
}
