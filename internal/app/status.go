package app

import "time"

// Status is whether a login can be used.
type Status int

// The statuses, from the best.
const (
	SignedIn  Status = iota
	Expiring         // signed in, and the sign-in or token expires within a week
	Expired          // the sign-in or token has expired
	SignedOut        // claude auth status fails
)

// expiringWithin is how soon an expiry makes a login Expiring.
const expiringWithin = 7 * 24 * time.Hour

func (s Status) String() string {
	return [...]string{"signed-in", "expiring", "expired", "signed-out"}[s]
}

// Usable reports whether claude can use the login: signed in, expiring or not.
func (s Status) Usable() bool { return s == SignedIn || s == Expiring }
