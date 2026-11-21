package quota

import (
	"testing"
)

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
