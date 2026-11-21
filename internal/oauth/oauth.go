// Package oauth signs a token login in: the OAuth authorization code flow
// with PKCE of Claude Code's client, the code pasted back by hand (ADR
// 0017). The values are the ones claude setup-token uses (claude 2.1.330);
// they are not documented.
package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// The client of Claude Code, and where its sign-in goes.
const (
	clientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	authorizeURL = "https://claude.com/cai/oauth/authorize"
	redirectURI  = "https://platform.claude.com/oauth/code/callback"
)

// Life is the life cc-login asks for, a year, as claude setup-token does.
const Life = 365 * 24 * time.Hour

// Scopes are what cc-login asks for: user:inference to run claude,
// user:profile so that claude can read the account and the organization.
var Scopes = []string{"user:inference", "user:profile"}

// Flow is one sign-in: the link to open, and what the code must match.
type Flow struct {
	URL      string
	verifier string
	state    string
}

// Start begins a sign-in.
func Start() Flow {
	f := Flow{verifier: random(), state: random()}

	sum := sha256.Sum256([]byte(f.verifier))
	q := url.Values{
		"code":                  {"true"},
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(Scopes, " ")},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"state":                 {f.state},
	}
	f.URL = authorizeURL + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")

	return f
}

// random is 32 random bytes, base64url without padding.
func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails (crypto/rand)
	return base64.RawURLEncoding.EncodeToString(b)
}

// CheckCode checks a pasted code: CODE#STATE, as the browser shows it.
func CheckCode(s string) error {
	code, state, ok := strings.Cut(s, "#")
	if !ok || code == "" || state == "" || strings.ContainsAny(s, " \t\r\n") {
		return errors.New("that is not the code the browser shows (CODE#STATE)")
	}
	return nil
}

// Token is what the token endpoint answers, without the refresh token,
// which cc-login does not keep.
type Token struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"` // seconds
	Scope       string `json:"scope"`      // space-separated
	Account     struct {
		Email string `json:"email_address"`
	} `json:"account"`
}

// Scopes are the scopes the token has.
func (t Token) Scopes() []string { return strings.Fields(t.Scope) }

// Life is how long the token lasts: Life when the server does not say.
func (t Token) Life() time.Duration {
	if t.ExpiresIn <= 0 {
		return Life
	}
	return time.Duration(t.ExpiresIn) * time.Second
}

// Client trades codes for tokens at BaseURL.
type Client struct {
	HTTP    *http.Client
	BaseURL string
}

// New is a client of https://platform.claude.com, or of
// $CC_LOGIN_OAUTH_URL (for tests).
func New() *Client {
	base := os.Getenv("CC_LOGIN_OAUTH_URL")
	if base == "" {
		base = "https://platform.claude.com"
	}
	return &Client{HTTP: &http.Client{}, BaseURL: base}
}

// Exchange trades code, pasted after sign-in f, for a token that lasts
// Life. Errors carry the server's description, never the code or a token.
func (c *Client) Exchange(ctx context.Context, f Flow, code string) (Token, error) {
	if err := CheckCode(code); err != nil {
		return Token{}, err
	}
	code, state, _ := strings.Cut(code, "#")
	if state != f.state {
		return Token{}, errors.New("the code is from another sign-in; open the link again")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	body, _ := json.Marshal(map[string]any{ // a map of strings and a number
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  redirectURI,
		"client_id":     clientID,
		"code_verifier": f.verifier,
		"state":         state,
		"expires_in":    int64(Life / time.Second),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/oauth/token", bytes.NewReader(body))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "cc-login")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return readToken(resp)
}

// readToken reads the answer of the token endpoint.
func readToken(resp *http.Response) (Token, error) {
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Token{}, fmt.Errorf("token exchange: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(b, &e) // a page that is not JSON leaves it empty
		if why := strings.Trim(e.Error+": "+e.Description, ": "); why != "" {
			return Token{}, fmt.Errorf("token exchange: HTTP %d: %s", resp.StatusCode, why)
		}
		return Token{}, fmt.Errorf("token exchange: HTTP %d", resp.StatusCode)
	}

	var t Token
	if err := json.Unmarshal(b, &t); err != nil {
		return Token{}, errors.New("token exchange: the answer is not JSON")
	}
	if !strings.HasPrefix(t.AccessToken, "sk-ant-oat") {
		return Token{}, errors.New("token exchange: the answer has no OAuth token")
	}

	return t, nil
}
