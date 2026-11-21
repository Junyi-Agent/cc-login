package main

import (
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func (c *cli) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of cc-login",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return c.say("cc-login %s (%s)", versionString(), goVersion())
		},
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
