package listing

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/quota"
)

func TestStatus(t *testing.T) {
	n := now.Unix()
	cases := []struct {
		l           Login
		state, tone string
	}{
		{Login{SignedIn: true}, "signed in", "ok"},
		{Login{SignedIn: true, Token: true}, "token", "ok"},
		{Login{SignedIn: true, Expiry: n + 86400}, "signed in", "warn"},
		{Login{SignedIn: true, Expiry: n + 8*86400}, "signed in", "ok"},
		{Login{SignedIn: true, Expiry: n}, "expired", "bad"},
		{Login{}, "signed out", "out"},
		{Login{Expiry: n - 1}, "expired", "bad"},
	}
	for _, c := range cases {
		if s, tone := Status(c.l, now); s != c.state || tone != c.tone {
			t.Errorf("Status(%+v) = %q %q, want %q %q", c.l, s, tone, c.state, c.tone)
		}
	}
}

func TestRenderJSON(t *testing.T) {
	n := now.Unix()
	logins := []Login{
		{Name: "work", Email: "me@company.com", Plan: "team", SignedIn: true, Expiry: n + 3600, QuotaOK: true,
			Quota: quota.Quota{FiveHour: quota.Window{Used: ip(27), Reset: n + 600}, SevenDay: quota.Window{Used: ip(120)}}},
		{Name: "tok", Email: "-", Plan: "-", SignedIn: true, Token: true, QuotaOK: true, Cached: true, CachedAt: n - 60,
			Quota: quota.Quota{SevenDay: quota.Window{Used: ip(5)}}},
		{Name: "key", Email: "API key …wxyz", Plan: "api", SignedIn: true, Key: true},
		{Name: "old", Email: "o@x", Plan: "pro", Expiry: n - 1, QuotaOK: true, Quota: quota.Quota{FiveHour: quota.Window{Used: ip(1)}}},
	}
	var b strings.Builder
	if err := RenderJSON(&b, logins, "work", now); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(b.String()), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b.String())
	}
	if len(got) != 4 {
		t.Fatalf("got %d logins", len(got))
	}
	work, tok, key, old := got[0], got[1], got[2], got[3]

	if work["default"] != true || work["kind"] != "sign-in" || work["status"] != "signed in" ||
		work["expires_at"] != "2026-11-20T13:00:00Z" || work["quota_source"] != "live" {
		t.Errorf("work: %v", work)
	}
	if fh, ok := work["five_hour"].(map[string]any); !ok || fh["left_percent"] != 73.0 || fh["resets_at"] != "2026-11-20T12:10:00Z" {
		t.Errorf("work five_hour: %v", fh)
	}
	if sd, ok := work["seven_day"].(map[string]any); !ok || sd["left_percent"] != 0.0 || sd["resets_at"] != nil {
		t.Errorf("work seven_day: over 100%% used should give 0 left: %v", sd)
	}
	if tok["kind"] != "token" || tok["account"] != nil || tok["quota_source"] != "cache" || tok["cached_at"] != "2026-11-20T11:59:00Z" || tok["five_hour"] != nil {
		t.Errorf("tok: %v", tok)
	}
	if key["kind"] != "api-key" || key["account"] != "API key …wxyz" || key["quota_source"] != nil {
		t.Errorf("key: %v", key)
	}
	if old["status"] != "expired" || old["five_hour"] != nil || old["quota_source"] != nil {
		t.Errorf("old: an expired login shows no quota: %v", old)
	}
}
