package quota

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	if err != nil || !at.Equal(time.Unix(999900, 0)) || *q.FiveHour.Used != 11 || !q.FiveHour.ResetsAt.Equal(time.Unix(1000500, 0)) {
		t.Fatalf("A: %+v %v %v", q, at, err)
	}
	if q.SevenDay.Used != nil {
		t.Error("A: the weekly window has reset since and should be left out")
	}

	if _, _, errB := Cached(path, "B", now); !errors.Is(errB, ErrNoCache) {
		t.Errorf("B: every window has reset: %v; want ErrNoCache", errB)
	}

	q, at, err = Cached(path, "C", now)
	if err != nil || !at.IsZero() || *q.SevenDay.Used != 5 || !q.SevenDay.ResetsAt.IsZero() {
		t.Errorf("C: %+v %v %v", q, at, err)
	}

	if _, _, errZ := Cached(path, "Z", now); !errors.Is(errZ, ErrNoCache) {
		t.Errorf("Z: no entry: %v; want ErrNoCache", errZ)
	}

	if _, _, errM := Cached(filepath.Join(t.TempDir(), "none.json"), "A", now); errM == nil {
		t.Error("no cache file: want an error")
	}
}

func TestLeft(t *testing.T) {
	n := func(i int) *int { return &i }

	cases := []struct {
		used *int
		want int
	}{
		{nil, 100},
		{n(0), 100},
		{n(27), 73},
		{n(100), 0},
		{n(130), 0},
		{n(-5), 100},
	}

	for _, tc := range cases {
		if got := (Window{Used: tc.used}).Left(); got != tc.want {
			t.Errorf("Left(%v) = %d; want %d", tc.used, got, tc.want)
		}
	}
}

func TestRound(t *testing.T) {
	f := 26.5
	if got := Round(&f); *got != 27 {
		t.Errorf("Round(26.5) = %d", *got)
	}
	if Round(nil) != nil {
		t.Error("Round(nil) is not nil")
	}
}

func TestCachePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}

	t.Setenv("CC_LOGIN_QUOTA_CACHE", "~/q.json")
	if got := CachePath(); got != filepath.Join(home, "q.json") {
		t.Errorf("CachePath = %q", got)
	}

	t.Setenv("CC_LOGIN_QUOTA_CACHE", "")
	if got := CachePath(); got != filepath.Join(home, ".claude", "quota-last.json") {
		t.Errorf("CachePath = %q", got)
	}
}
