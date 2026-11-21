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
	if *q.FiveHour.Used != 27 || !q.FiveHour.ResetsAt.Equal(reset) {
		t.Errorf("five_hour = %d %v", *q.FiveHour.Used, q.FiveHour.ResetsAt)
	}
	if *q.SevenDay.Used != 76 {
		t.Errorf("seven_day = %d", *q.SevenDay.Used)
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
	if err != nil || q.FiveHour.Used != nil || *q.SevenDay.Used != 3 || !q.SevenDay.ResetsAt.IsZero() {
		t.Fatalf("q = %+v, %v", q, err)
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
