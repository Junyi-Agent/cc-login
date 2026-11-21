package creds

import (
	"fmt"
	"testing"
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

func TestNewest(t *testing.T) {
	cases := []struct {
		name  string
		blobs [][]byte
		tok   string
		exp   int64
	}{
		{"nothing stored", nil, "", 0},
		{"the file", [][]byte{cred("from-file", 1_000_600)}, "from-file", 1_000_600},
		{"the later one wins", [][]byte{cred("from-file", 1_000_600), cred("from-keychain", 1_003_600)}, "from-keychain", 1_003_600},
		{"no Keychain item", [][]byte{cred("from-file", 1_000_600), nil}, "from-file", 1_000_600},
		{"an empty token", [][]byte{[]byte(`{"claudeAiOauth":{"accessToken":""}}`)}, "", 0},
		{"not JSON", [][]byte{[]byte("nope")}, "", 0},
	}

	for _, tc := range cases {
		tok, exp := newest(tc.blobs...)
		if tok != tc.tok || (tok != "" && exp.Unix() != tc.exp) {
			t.Errorf("%s: %q expiring %d; want %q, %d", tc.name, tok, exp.Unix(), tc.tok, tc.exp)
		}
	}
}
