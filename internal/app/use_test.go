package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

func TestUse(t *testing.T) {
	a, _ := three(t)
	ctx := context.Background()

	if err := a.Use(ctx, "ci"); err != nil {
		t.Fatal(err)
	}
	if st := load(t, a); st.Default != "t1" {
		t.Errorf("default = %q, want t1", st.Default)
	}

	// api is signed out in the fake: it cannot be the default.
	if err := a.Use(ctx, "api"); err == nil || !strings.Contains(err.Error(), "signed out") {
		t.Errorf("use api = %v; want signed out", err)
	}
	if err := a.Use(ctx, "nope"); !errors.Is(err, state.ErrNoLogin) {
		t.Errorf("use nope = %v; want ErrNoLogin", err)
	}
	if st := load(t, a); st.Default != "t1" {
		t.Errorf("a failed use changed the default to %q", st.Default)
	}
}
