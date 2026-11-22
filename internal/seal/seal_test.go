package seal

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func init() { Iterations = minIterations } // fast tests

func TestRoundTrip(t *testing.T) {
	plain := []byte(`{"secret":"sk-ant-oat01-fake"}`)

	sealed, err := Seal("correct horse", plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("sk-ant")) {
		t.Fatal("the sealed file holds the plain text")
	}

	got, err := Open("correct horse", sealed)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("Open = %q, %v; want %q", got, err, plain)
	}
}

// Two seals of the same data differ: a new salt and nonce each time.
func TestSealIsRandom(t *testing.T) {
	a, _ := Seal("pw-pw-pw-pw", []byte("x"))
	b, _ := Seal("pw-pw-pw-pw", []byte("x"))
	if bytes.Equal(a, b) {
		t.Error("two seals are the same")
	}
}

func TestOpenRefuses(t *testing.T) {
	sealed, err := Seal("correct horse", []byte("data"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		passphrase string
		change     func(e map[string]any)
		want       string
	}{
		{"a wrong passphrase", "wrong horse", nil, ErrOpen.Error()},
		{"more iterations", "correct horse", func(e map[string]any) { e["iterations"] = minIterations + 1 }, ErrOpen.Error()},
		{"too many iterations", "correct horse", func(e map[string]any) { e["iterations"] = maxIterations + 1 }, "PBKDF2 iterations"},
		{"another kdf", "correct horse", func(e map[string]any) { e["kdf"] = "argon2id" }, "this cc-login reads"},
		{"another format", "correct horse", func(e map[string]any) { e["cc_login_export"] = 2 }, "this cc-login reads"},
		{"changed data", "correct horse", func(e map[string]any) { e["data"] = "AAAAAAAAAAAAAAAAAAAAAAAA" }, ErrOpen.Error()},
		{"no nonce", "correct horse", func(e map[string]any) { delete(e, "nonce") }, ErrOpen.Error()},
	}

	for _, tc := range cases {
		file := sealed
		if tc.change != nil {
			file = changed(t, sealed, tc.change)
		}

		_, err := Open(tc.passphrase, file)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Open = %v; want %q", tc.name, err, tc.want)
		}
	}
}

func TestOpenNotAnExport(t *testing.T) {
	for _, file := range []string{"", "nope", `{"version": 1}`} {
		if _, err := Open("pw", []byte(file)); err == nil || errors.Is(err, ErrOpen) {
			t.Errorf("Open(%q) = %v; want not an export file", file, err)
		}
	}
}

// changed is sealed with change applied to its JSON.
func changed(t *testing.T, sealed []byte, change func(map[string]any)) []byte {
	t.Helper()

	var e map[string]any
	if err := json.Unmarshal(sealed, &e); err != nil {
		t.Fatal(err)
	}
	change(e)

	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
