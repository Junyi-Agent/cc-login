package main

import (
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func (c *cli) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of cc-login, and how claude starts",
		Long: `Print the version of cc-login, the claude it runs, and whether a plain claude
goes through cc-login: whether the claude link is in place.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return c.version() },
	}
}

// version prints the version of cc-login, the claude it runs, and what a
// plain claude runs: whether the claude link is in place.
func (c *cli) version() error {
	if err := c.print("cc-login %s (%s)", versionString(), goVersion()); err != nil {
		return err
	}

	s := c.app.Setup()
	claudeLine := "claude: " + s.Claude
	if s.ClaudeErr != nil {
		claudeLine = "claude: " + s.ClaudeErr.Error()
	}
	if err := c.print("%s", claudeLine); err != nil {
		return err
	}

	switch {
	case s.Linked:
		return c.print("claude link: %s; a plain claude runs with the default login", s.InPath)
	case s.InPath == "":
		return c.print("claude link: none; PATH has no claude")
	default:
		return c.print("claude link: none; a plain claude runs %s without cc-login (README: Getting Started)", s.InPath)
	}
}

// version is set by goreleaser (-X main.version=...).
var version = ""

// versionString is the version: goreleaser's, else the one go build or go
// install records ("v0.2.0", or "v0.0.0-20261121...-1a2b3c4d5e6f+dirty" for
// a local build).
func versionString() string {
	if version != "" {
		return version
	}

	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}

// goVersion is the Go release the binary was built with.
func goVersion() string { return runtime.Version() }
