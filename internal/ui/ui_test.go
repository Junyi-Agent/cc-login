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

	"github.com/Junyi-Agent/cc-login/internal/app"
	"github.com/Junyi-Agent/cc-login/internal/quota"
	"github.com/Junyi-Agent/cc-login/internal/state"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

var now = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func pct(n int) *int { return &n }

// views are one login of each kind and status.
func views() []app.View {
	at := func(h int) time.Time { return now.Add(time.Duration(h) * time.Hour) }
	return []app.View{
		{
			Name: "work", Default: true, Kind: state.SignIn, Account: "me@example.com", Plan: "team",
			Status: app.SignedIn, ExpiresAt: at(30 * 24), QuotaSource: app.QuotaLive,
			Quota: quota.Quota{
				FiveHour: quota.Window{Used: pct(30), ResetsAt: at(3)},
				SevenDay: quota.Window{Used: pct(85), ResetsAt: at(50)},
			},
		},
		{
			Name: "soon", Kind: state.SignIn, Account: "a-long-account-name@example.com", Plan: "max",
			Status: app.Expiring, ExpiresAt: at(30), QuotaSource: app.QuotaCache, CachedAt: at(-2),
			Quota: quota.Quota{FiveHour: quota.Window{Used: pct(60)}},
		},
		{Name: "old", Kind: state.SignIn, Status: app.Expired, ExpiresAt: at(-5)},
		{Name: "gone", Kind: state.SignIn, Account: "x@example.com", Status: app.SignedOut},
		{Name: "ci", Kind: state.Token, Status: app.SignedIn, ExpiresAt: at(400 * 24), QuotaGap: app.GapToken},
		{Name: "api", Kind: state.APIKey, Account: "API key …wxyz", Plan: "api", Status: app.SignedIn, QuotaGap: app.GapNoLogin},
		{Name: "stale", Kind: state.SignIn, Status: app.SignedIn, QuotaGap: app.GapStale},
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

// A narrow terminal drops the date column, then shortens the accounts.
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

func TestFormatJSON(t *testing.T) {
	var b bytes.Buffer
	if err := Format(&b, views(), "json"); err != nil {
		t.Fatal(err)
	}

	var got []Login
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	work, ci := got[0], got[4]
	if work.Status != "signed-in" || work.Kind != "sign-in" || !work.Default || *work.QuotaSource != "live" {
		t.Errorf("work = %+v", work)
	}
	if work.FiveHour.LeftPercent != 70 || work.SevenDay.LeftPercent != 15 {
		t.Errorf("work quota = %+v %+v", work.FiveHour, work.SevenDay)
	}
	if ci.Kind != "token" || ci.FiveHour != nil || ci.QuotaSource != nil || ci.Plan != nil {
		t.Errorf("ci = %+v", ci)
	}
}

func TestFormatTSVAndTemplate(t *testing.T) {
	cases := []struct {
		format, first string
	}{
		{"tsv", "work\ttrue\tsign-in\tsigned-in\tme@example.com\tteam\t2026-12-21T12:00:00Z\t70\t15"},
		{"{{.Name}} {{.Status}}", "work signed-in"},
	}

	for _, tc := range cases {
		var b bytes.Buffer
		if err := Format(&b, views(), tc.format); err != nil {
			t.Fatal(err)
		}

		lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
		if len(lines) != len(views()) || lines[0] != tc.first {
			t.Errorf("%s: %d lines, first %q; want %d, %q", tc.format, len(lines), lines[0], len(views()), tc.first)
		}
	}

	if err := Format(&bytes.Buffer{}, views(), "{{.Nope"); err == nil {
		t.Error("a broken template: want an error")
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
