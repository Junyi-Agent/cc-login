package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A key or token typed or pasted into the input box shows as *, one per
// character, never as itself.
func TestSecretInputMasks(t *testing.T) {
	const pasted = "sk-ant-api03-abcd"
	var v string
	in := secretInput("API key", "help", func(string) error { return nil }, &v)
	in.Focus()

	in.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(pasted), Paste: true})
	view := in.View()

	if strings.Contains(view, pasted) || strings.Contains(view, "abcd") {
		t.Errorf("the input box shows the key:\n%s", view)
	}
	if !strings.Contains(view, strings.Repeat("*", len(pasted))) {
		t.Errorf("the input box does not show %d *:\n%s", len(pasted), view)
	}
}
