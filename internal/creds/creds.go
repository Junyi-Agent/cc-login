// Package creds reads the OAuth access token of a sign-in, which Claude Code
// keeps in <dir>/.credentials.json, or on macOS in the Keychain item
// "Claude Code-credentials-<hash>", hash being the first 8 hex digits of the
// SHA-256 of the directory's path. Claude Code does not document that name.
package creds

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// KeychainService is the Keychain item name that Claude Code uses for the
// credentials directory dir.
func KeychainService(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return "Claude Code-credentials-" + hex.EncodeToString(sum[:])[:8]
}

type credFile struct {
	ClaudeAiOauth *struct {
		AccessToken string  `json:"accessToken"`
		ExpiresAt   float64 `json:"expiresAt"` // milliseconds since the epoch
	} `json:"claudeAiOauth"`
}

// ReadKeychain returns the password of the generic Keychain item service.
// It is a variable so that tests and non-macOS systems can replace it.
var ReadKeychain = func(ctx context.Context, service string) ([]byte, bool) {
	if runtime.GOOS != "darwin" {
		return nil, false
	}

	out, err := exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-w").Output()
	if err != nil {
		return nil, false
	}

	return out, true
}

// Result says why there is no usable access token.
type Result int

// The results of AccessToken.
const (
	OK      Result = iota // a valid access token
	None                  // no sign-in stored, or no token in it
	Expired               // the newest access token has expired
)

// AccessToken returns the access token of the sign-in stored in dir. When
// both the file and the Keychain hold one, the one that expires later is the
// current one. A token counts as expired 30 seconds before its expiry.
func AccessToken(ctx context.Context, dir string, now time.Time) (string, Result) {
	var blobs [][]byte
	if b, err := os.ReadFile(filepath.Join(dir, ".credentials.json")); err == nil {
		blobs = append(blobs, b)
	}
	if b, ok := ReadKeychain(ctx, KeychainService(dir)); ok {
		blobs = append(blobs, b)
	}

	// The token that expires last.
	var best string
	var bestExp int64
	for _, b := range blobs {
		var c credFile
		if json.Unmarshal(b, &c) != nil || c.ClaudeAiOauth == nil {
			continue
		}
		exp := int64(c.ClaudeAiOauth.ExpiresAt / 1000)
		if c.ClaudeAiOauth.AccessToken != "" && exp > bestExp {
			best, bestExp = c.ClaudeAiOauth.AccessToken, exp
		}
	}

	if best == "" {
		return "", None
	}
	if bestExp <= now.Unix()+30 {
		return "", Expired
	}
	return best, OK
}
