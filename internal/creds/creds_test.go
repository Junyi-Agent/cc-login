package creds

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func cred(tok string, expSec int64) []byte {
	return []byte(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d}}`, tok, expSec*1000))
}

func TestKeychainService(t *testing.T) {
	// printf %s /x | shasum -a 256 → b3d1db31...
	if got := KeychainService("/x"); got != "Claude Code-credentials-b3d1db31" {
		t.Fatalf("KeychainService = %q", got)
	}
}

func TestAccessToken(t *testing.T) {
	now := time.Unix(1_000_000, 0)

	var keychain []byte
	old := ReadKeychain
	ReadKeychain = func(context.Context, string) ([]byte, bool) { return keychain, keychain != nil }
	defer func() { ReadKeychain = old }()

	dir := t.TempDir()
	file := filepath.Join(dir, ".credentials.json")
	ctx := context.Background()

	if _, r := AccessToken(ctx, dir, now); r != None {
		t.Fatalf("nothing stored: %v", r)
	}

	if err := os.WriteFile(file, cred("from-file", 1_000_600), 0o600); err != nil {
		t.Fatal(err)
	}
	if tok, r := AccessToken(ctx, dir, now); r != OK || tok != "from-file" {
		t.Fatalf("file: %q %v", tok, r)
	}

	keychain = cred("from-keychain", 1_003_600)
	if tok, _ := AccessToken(ctx, dir, now); tok != "from-keychain" {
		t.Fatalf("the later one should win: %q", tok)
	}

	keychain = nil
	if err := os.WriteFile(file, cred("soon", 1_000_020), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, r := AccessToken(ctx, dir, now); r != Expired {
		t.Fatalf("within 30s of expiry counts as expired: %v", r)
	}

	if err := os.WriteFile(file, []byte(`{"claudeAiOauth":{"accessToken":""}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, r := AccessToken(ctx, dir, now); r != None {
		t.Fatalf("empty token: %v", r)
	}
}
