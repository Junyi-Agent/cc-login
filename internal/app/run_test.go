package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

func TestRun(t *testing.T) {
	cases := []struct {
		name    string
		env     []string
		want    string // the ID of the login, "" for an error
		key     string // ANTHROPIC_API_KEY in claude's environment
		wantErr string
	}{
		{name: "", want: "a1"},
		{name: "api", want: "k1", key: "secret-of-k1"},
		{name: "", env: []string{"CC_LOGIN_ID=t1"}, want: "t1"},
		{name: "", env: []string{"CC_LOGIN_ID=gone"}, want: "a1"},
		{name: "work", env: []string{"CC_LOGIN_ID=t1"}, want: "a1"},
		{name: "nope", wantErr: "no such login"},
		{name: "", env: []string{"CC_LOGIN_DEPTH=10"}, wantErr: "started itself"},
	}

	for _, tc := range cases {
		a, _ := three(t)
		a.Env = append(a.Env, tc.env...)

		x, err := a.Run(tc.name)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Run(%q) %v = %v; want %q", tc.name, tc.env, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Run(%q) %v: %v", tc.name, tc.env, err)
		}

		id := lookup(x.Env, "CC_LOGIN_ID")
		if id != tc.want || lookup(x.Env, "ANTHROPIC_API_KEY") != tc.key {
			t.Errorf("Run(%q) %v = %s with key %q; want %s, %q", tc.name, tc.env, id, lookup(x.Env, "ANTHROPIC_API_KEY"), tc.want, tc.key)
		}
		if got := lookup(x.Env, "CC_LOGIN_DEPTH"); got == "" || filepath.Base(lookup(x.Env, "CLAUDE_SECURESTORAGE_CONFIG_DIR")) != lookup(x.Env, "CC_LOGIN_ID") {
			t.Errorf("Run(%q): depth %q, id %q", tc.name, got, lookup(x.Env, "CC_LOGIN_ID"))
		}
	}

	a, _ := three(t)
	if err := a.Store.Update(func(st *state.State) error { st.Default = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Run(""); !errors.Is(err, ErrNoDefault) {
		t.Errorf("Run with no default = %v", err)
	}
}
