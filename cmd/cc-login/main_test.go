package main

import (
	"os"
	"testing"
)

// The argument checks fail before anything touches the data directory.
func TestRunRejectsBadArguments(t *testing.T) {
	t.Setenv("CC_LOGIN_ROOT", t.TempDir()) // not set up: a run that got past the checks fails too
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devnull.Close() }()
	stderr := os.Stderr
	os.Stderr = devnull
	defer func() { os.Stderr = stderr }()

	for _, args := range [][]string{
		{"nope"}, {"-x"}, {"list", "extra"}, {"list", "--yaml"}, {"use", "a"},
	} {
		if code := run(args); code != 1 {
			t.Errorf("run(%q) = %d, want 1", args, code)
		}
	}
}

func TestRunNotSetUp(t *testing.T) {
	t.Setenv("CC_LOGIN_ROOT", t.TempDir())
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	defer func() { _ = devnull.Close() }()
	stderr := os.Stderr
	os.Stderr = devnull
	defer func() { os.Stderr = stderr }()

	if code := run([]string{"list", "--json"}); code != 1 {
		t.Errorf("list on a machine that is not set up = %d, want 1", code)
	}
}
