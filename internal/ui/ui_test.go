package ui

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Junyi-Agent/cc-login/internal/anthropic"
	"github.com/Junyi-Agent/cc-login/internal/app"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

var now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

// usedWindow is a quota window with that percent used, resetting at reset
// (none when nil).
func usedWindow(used float64, reset *time.Time) *anthropic.Window {
	return &anthropic.Window{Used: &used, ResetsAt: reset}
}

// views are one login of each kind and status.
func views() []app.View {
	at := func(h int) time.Time { return now.Add(time.Duration(h) * time.Hour) }
	in := func(h int) *time.Time { t := at(h); return &t }

	return []app.View{
		{
			Name: "work", Default: true, Kind: state.SignIn, Account: "me@example.com", Plan: "team",
			Status: app.SignedIn, ExpiresAt: at(30 * 24),
			Quota: &anthropic.Quota{FiveHour: usedWindow(30, in(3)), SevenDay: usedWindow(85, in(50))},
		},
		{
			Name: "soon", Kind: state.SignIn, Account: "a-long-account-name@example.com", Plan: "max",
			Status: app.Expiring, ExpiresAt: at(30),
			Quota: &anthropic.Quota{FiveHour: usedWindow(60, nil)},
		},
		{Name: "old", Kind: state.SignIn, Status: app.Expired, ExpiresAt: at(-5)},
		{Name: "gone", Kind: state.SignIn, Account: "x@example.com", Status: app.SignedOut},
		{Name: "ci", Kind: state.Token, Status: app.SignedIn, ExpiresAt: at(400 * 24)},
		{Name: "api", Kind: state.APIKey, Account: "API key …wxyz", Plan: "api", Status: app.SignedIn},
		{Name: "stale", Kind: state.SignIn, Status: app.SignedIn, QuotaErr: app.ErrStaleToken},
	}
}

// golden compares got with testdata/name, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)

	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s differs (go test -update rewrites it):\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func TestMain(m *testing.M) {
	time.Local = time.UTC // the table prints local times
	os.Exit(m.Run())
}

// The table to a file has no colors and no trailing spaces.
func TestTable(t *testing.T) {
	var b bytes.Buffer
	if err := Table(&b, views(), Options{Now: now}); err != nil {
		t.Fatal(err)
	}

	golden(t, "table.golden", b.String())

	for i, line := range strings.Split(b.String(), "\n") {
		if strings.HasSuffix(line, " ") || strings.Contains(line, "\x1b") {
			t.Errorf("line %d has a trailing space or a color: %q", i+1, line)
		}
	}
}

// A narrow terminal drops the date column.
func TestTableNarrow(t *testing.T) {
	var b bytes.Buffer
	if err := Table(&b, views(), Options{Now: now, Width: 90}); err != nil {
		t.Fatal(err)
	}

	golden(t, "table-narrow.golden", b.String())

	if strings.Contains(b.String(), "12-21 12:00") {
		t.Error("the date column is still there")
	}
}

func TestTableEmpty(t *testing.T) {
	var b bytes.Buffer
	if err := Table(&b, nil, Options{Now: now}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(b.String(), "cc-login add NAME") {
		t.Errorf("empty table = %q", b.String())
	}
}

func TestJSON(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, views()); err != nil {
		t.Fatal(err)
	}

	var got []Login
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	work, ci := got[0], got[4]
	if work.Status != "signed-in" || work.Kind != "sign-in" || !work.Default {
		t.Errorf("work = %+v", work)
	}
	if work.FiveHour.LeftPercent != 70 || work.SevenDay.LeftPercent != 15 || !work.FiveHour.ResetsAt.Equal(now.Add(3*time.Hour)) {
		t.Errorf("work quota = %+v %+v", work.FiveHour, work.SevenDay)
	}
	if soon := got[1]; soon.FiveHour.ResetsAt != nil || soon.SevenDay != nil {
		t.Errorf("soon quota = %+v %+v; want no reset time and no weekly window", soon.FiveHour, soon.SevenDay)
	}
	if ci.Kind != "token" || ci.FiveHour != nil || ci.Plan != nil {
		t.Errorf("ci = %+v", ci)
	}
}

func TestAgo(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "1m"},
		{59 * time.Minute, "59m"},
		{3 * time.Hour, "3h"},
		{47 * time.Hour, "47h"},
		{72 * time.Hour, "3d"},
	}

	for _, tc := range cases {
		if got := Ago(tc.d); got != tc.want {
			t.Errorf("Ago(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
