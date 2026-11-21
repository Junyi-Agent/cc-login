package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	f := Start()

	u, err := url.Parse(f.URL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()

	sum := sha256.Sum256([]byte(f.verifier))
	want := map[string]string{
		"client_id":             clientID,
		"redirect_uri":          redirectURI,
		"scope":                 "user:inference user:profile",
		"code_challenge":        base64.RawURLEncoding.EncodeToString(sum[:]),
		"code_challenge_method": "S256",
		"state":                 f.state,
		"response_type":         "code",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q; want %q", k, q.Get(k), v)
		}
	}
	if strings.Contains(f.URL, "+") {
		t.Error("spaces in the URL are + ; want %20")
	}

	if g := Start(); g.state == f.state || g.verifier == f.verifier {
		t.Error("two sign-ins share a state or a verifier")
	}
}

func TestCheckCode(t *testing.T) {
	cases := []struct {
		code string
		ok   bool
	}{
		{"abc#def", true},
		{"abc", false},
		{"#def", false},
		{"abc#", false},
		{"ab c#def", false},
		{"sk-ant-oat01-a-token", false},
	}

	for _, tc := range cases {
		if err := CheckCode(tc.code); (err == nil) != tc.ok {
			t.Errorf("CheckCode(%q) = %v; want ok %t", tc.code, err, tc.ok)
		}
	}
}

// serve is a Client of a test server that answers with h.
func serve(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), BaseURL: srv.URL}
}

const answer = `{"token_type":"Bearer","access_token":"sk-ant-oat01-new","expires_in":31536000,
	"refresh_token":"sk-ant-ort01-dropped","scope":"user:inference user:profile",
	"account":{"uuid":"u1","email_address":"me@example.com"},"organization":{"uuid":"o1","name":"Org"}}`

func TestExchange(t *testing.T) {
	f := Start()
	var got map[string]any
	c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/oauth/token" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(answer))
	})

	tok, err := c.Exchange(context.Background(), f, "the-code#"+f.state)
	if err != nil {
		t.Fatal(err)
	}

	if tok.AccessToken != "sk-ant-oat01-new" || tok.Account.Email != "me@example.com" || tok.Life() != Life {
		t.Errorf("token = %+v", tok)
	}
	if s := tok.Scopes(); len(s) != 2 || s[1] != "user:profile" {
		t.Errorf("scopes = %q", s)
	}

	want := map[string]any{
		"grant_type": "authorization_code", "code": "the-code", "state": f.state,
		"code_verifier": f.verifier, "client_id": clientID, "redirect_uri": redirectURI,
		"expires_in": float64(31536000),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("request %s = %v; want %v", k, got[k], v)
		}
	}
}

// A code of another sign-in, a refusal or a strange answer is an error
// that shows neither the code nor a token.
func TestExchangeFails(t *testing.T) {
	cases := []struct {
		name   string
		code   string // "" is a good code
		status int
		body   string
		want   string
	}{
		{"another sign-in", "c0de-xyz#other-state", 200, answer, "another sign-in"},
		{"refused", "", 400, `{"error":"invalid_grant","error_description":"Invalid 'code' in request."}`, "HTTP 400: invalid_grant: Invalid 'code'"},
		{"a page", "", 403, `<html>blocked</html>`, "HTTP 403"},
		{"not JSON", "", 200, `nope`, "not JSON"},
		{"no token", "", 200, `{"access_token":"sk-ant-api03-x"}`, "no OAuth token"},
	}

	for _, tc := range cases {
		f := Start()
		c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		code := tc.code
		if code == "" {
			code = "c0de-xyz#" + f.state
		}

		_, err := c.Exchange(context.Background(), f, code)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v; want %q", tc.name, err, tc.want)
			continue
		}
		if strings.Contains(err.Error(), "c0de-xyz") || strings.Contains(err.Error(), "sk-ant") {
			t.Errorf("%s: the error shows a secret: %v", tc.name, err)
		}
	}
}

func TestTokenLife(t *testing.T) {
	if got := (Token{ExpiresIn: 3600}).Life(); got != time.Hour {
		t.Errorf("Life = %v; want the server's", got)
	}
	if got := (Token{}).Life(); got != Life {
		t.Errorf("Life = %v; want a year when the server does not say", got)
	}
}

func TestNewBaseURL(t *testing.T) {
	t.Setenv("CC_LOGIN_OAUTH_URL", "")
	if got := New().BaseURL; got != "https://platform.claude.com" {
		t.Errorf("BaseURL = %q", got)
	}

	t.Setenv("CC_LOGIN_OAUTH_URL", "http://127.0.0.1:9")
	if got := New().BaseURL; got != "http://127.0.0.1:9" {
		t.Errorf("BaseURL = %q", got)
	}
}
