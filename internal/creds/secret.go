package creds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// file is the file that holds the secret of a login of that kind.
func file(kind state.Kind) (string, error) {
	switch kind {
	case state.APIKey:
		return state.APIKeyFile, nil
	case state.Token:
		return state.TokenFile, nil
	case state.SignIn:
	}
	return "", fmt.Errorf("a sign-in login has no key or token file")
}

// Secret reads the API key or the token of the login in credentials
// directory dir. The error names the file, never its content.
func Secret(dir string, kind state.Kind) (string, error) {
	name, err := file(kind)
	if err != nil {
		return "", err
	}

	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(b)), nil
}

// WriteSecret saves the API key or the token of the login in credentials
// directory dir in a 0600 file. The old secret stays whole until the new one
// is (state.WriteAtomic).
func WriteSecret(dir string, kind state.Kind, secret string) error {
	name, err := file(kind)
	if err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(dir, name), []byte(secret+"\n"))
}
