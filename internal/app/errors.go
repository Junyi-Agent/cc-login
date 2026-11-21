package app

import "errors"

// Errors of the commands. A login that does not exist is state.ErrNoLogin.
var (
	ErrNoDefault = errors.New("no default login; choose one with: cc-login use NAME")
	ErrCancelled = errors.New("nothing changed")

	// ErrStaleToken means the sign-in's access token has expired, so list
	// cannot read the quota.
	ErrStaleToken = errors.New("the access token expired; a claude run with the login refreshes it")
)
