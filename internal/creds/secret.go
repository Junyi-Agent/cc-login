package creds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Junyi-Agent/cc-login/internal/state"
)

// Secret reads the API key or the token of the login in credentials
// directory dir. The error names the file, never its content.
func Secret(dir string, kind state.Kind) (string, error) {
	var name string
	switch kind {
	case state.APIKey:
		name = state.APIKeyFile
	case state.Token:
		name = state.TokenFile
	case state.SignIn:
		return "", fmt.Errorf("a sign-in login has no key or token file")
	}

	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(b)), nil
}
