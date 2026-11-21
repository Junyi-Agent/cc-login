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
	if err := c.say("cc-login %s (%s)", versionString(), goVersion()); err != nil {
		return err
	}

	s := c.app.Setup()
	claudeLine := "claude: " + s.Claude
	if s.ClaudeErr != nil {
		claudeLine = "claude: " + s.ClaudeErr.Error()
	}
	if err := c.say("%s", claudeLine); err != nil {
		return err
	}

	switch {
	case s.Linked:
		return c.say("claude link: %s; a plain claude runs with the default login", s.InPath)
	case s.InPath == "":
		return c.say("claude link: none; PATH has no claude")
	default:
		return c.say("claude link: none; a plain claude runs %s without cc-login (README: Getting Started)", s.InPath)
	}
}

// version is set by goreleaser (-X main.version=...); a go install keeps it
// in the build info instead.
var version = ""

// versionString is the version, and the commit when the build knows it:
// "v0.2.0", or "(devel) 1a2b3c4d5e6f-dirty" for a local build.
func versionString() string {
	v := version
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return orUnknown(v)
	}
	if v == "" {
		v = info.Main.Version
	}

	var rev, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if rev != "" {
		v += " " + rev + dirty
	}

	return orUnknown(v)
}

func orUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}

// goVersion is the Go release the binary was built with.
func goVersion() string { return runtime.Version() }
