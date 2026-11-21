package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// serve points UsageURL at a test server with handler h until the test ends.
func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	old := UsageURL
	UsageURL = srv.URL
	t.Cleanup(func() {
		UsageURL = old
		srv.Close()
	})
	return srv
}

func TestFetch(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" || r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":26.5,"resets_at":"2026-11-21T05:00:00.123456+00:00"},
			"seven_day":{"utilization":76.4,"resets_at":"2026-11-25T00:00:00Z"},"seven_day_opus":null}`))
	})

	q, err := Fetch(context.Background(), srv.Client(), "good")
	if err != nil {
		t.Fatal(err)
	}

	if *q.FiveHour.Used != 27 || q.FiveHour.Reset != time.Date(2026, 11, 21, 5, 0, 0, 0, time.UTC).Unix() {
		t.Errorf("five_hour = %d %d", *q.FiveHour.Used, q.FiveHour.Reset)
	}
	if *q.SevenDay.Used != 76 {
		t.Errorf("seven_day = %d", *q.SevenDay.Used)
	}

	if _, err := Fetch(context.Background(), srv.Client(), "bad"); err == nil {
		t.Error("401: want an error")
	}
}

func TestFetchNoLimit(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"five_hour":null,"seven_day":{"utilization":3,"resets_at":null}}`))
	})

	q, err := Fetch(context.Background(), srv.Client(), "x")
	if err != nil || q.FiveHour.Used != nil || *q.SevenDay.Used != 3 || q.SevenDay.Reset != 0 {
		t.Fatalf("q = %+v, %v", q, err)
	}
}

func TestCached(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	path := filepath.Join(t.TempDir(), "cache.json")
	if err := os.WriteFile(path, []byte(`{
		"acct/org": {"five_hour": {"used_percentage": 1}},
		"dir:A": {"five_hour": {"used_percentage": 10.6, "resets_at": 1000500},
		          "seven_day": {"used_percentage": 40, "resets_at": 999000}, "at": 999900},
		"dir:B": {"five_hour": {"used_percentage": 10, "resets_at": 900000}},
		"dir:C": {"seven_day": {"used_percentage": 5}}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	q, at, err := Cached(path, "A", now)
	if err != nil || at != 999900 || *q.FiveHour.Used != 11 || q.FiveHour.Reset != 1000500 {
		t.Fatalf("A: %+v %d %v", q, at, err)
	}
	if q.SevenDay.Used != nil {
		t.Error("A: the weekly window has reset since and should be left out")
	}

	if _, _, errB := Cached(path, "B", now); errB == nil {
		t.Error("B: every window has reset; want an error")
	}

	q, at, err = Cached(path, "C", now)
	if err != nil || at != 0 || *q.SevenDay.Used != 5 || q.SevenDay.Reset != 0 {
		t.Errorf("C: %+v %d %v", q, at, err)
	}

	if _, _, errZ := Cached(path, "Z", now); errZ == nil {
		t.Error("Z: no entry; want an error")
	}
}
