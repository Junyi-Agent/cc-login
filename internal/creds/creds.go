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

// readKeychain returns the password of the generic Keychain item service;
// nil off macOS or when there is none.
func readKeychain(ctx context.Context, service string) []byte {
	if runtime.GOOS != "darwin" {
		return nil
	}

	out, err := exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-w").Output()
	if err != nil {
		return nil
	}
	return out
}

// AccessToken is the access token of the sign-in stored for credentials
// directory dir, in .credentials.json or the Keychain, and when it expires;
// "" when none is stored.
func AccessToken(ctx context.Context, dir string) (string, time.Time) {
	file, _ := os.ReadFile(filepath.Join(dir, ".credentials.json"))
	return newest(file, readKeychain(ctx, KeychainService(dir)))
}

// newest is the access token in blobs that expires last, and its expiry. Both
// the file and the Keychain can hold one; the later one is the current one.
func newest(blobs ...[]byte) (string, time.Time) {
	var tok string
	var exp time.Time

	for _, b := range blobs {
		var c credFile
		if json.Unmarshal(b, &c) != nil || c.ClaudeAiOauth == nil || c.ClaudeAiOauth.AccessToken == "" {
			continue
		}

		t := time.UnixMilli(int64(c.ClaudeAiOauth.ExpiresAt))
		if tok == "" || t.After(exp) {
			tok, exp = c.ClaudeAiOauth.AccessToken, t
		}
	}

	return tok, exp
}
