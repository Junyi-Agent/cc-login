// Package quota is the plan quota that claude's /usage shows.
package quota

import (
	"math"
	"time"
)

// Window is one quota window. Used is nil when the plan has no such limit.
type Window struct {
	Used     *int      // percent used, rounded
	ResetsAt time.Time // zero when unknown
}

// Left is the percent left, 0 to 100.
func (w Window) Left() int {
	if w.Used == nil {
		return 100
	}
	return max(0, min(100, 100-*w.Used))
}

// Quota is the 5-hour and the weekly window.
type Quota struct {
	FiveHour Window
	SevenDay Window
}

// Any reports whether either window has a value.
func (q Quota) Any() bool { return q.FiveHour.Used != nil || q.SevenDay.Used != nil }

// Round is f rounded to a whole percent; nil stays nil.
func Round(f *float64) *int {
	if f == nil {
		return nil
	}

	n := int(math.Round(*f))
	return &n
}
