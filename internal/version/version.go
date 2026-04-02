// Package version provides build-time version information for the ytr binary.
// Variables are set via ldflags at build time and default to development values.
// When built via go install (no ldflags), version info is extracted from
// debug.ReadBuildInfo as a fallback.
package version

import (
	"runtime"
	"runtime/debug"
	"strings"
)

// Build-time variables set via ldflags.
// Example: go build -ldflags="-X github.com/slavkluev/ytr/internal/version.Version=1.0.0".
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func init() {
	InitFromBuildInfo()
}

// InitFromBuildInfo populates Version, Commit, and Date from debug.ReadBuildInfo
// when ldflags are not set (e.g., go install). If Version is already set by
// ldflags (GoReleaser build), this function is a no-op.
func InitFromBuildInfo() {
	if Version != "dev" {
		return // ldflags were set (GoReleaser build), use them
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	// go install sets Main.Version to the module version (e.g., "v0.1.0")
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = strings.TrimPrefix(info.Main.Version, "v")
	}

	// VCS info from go install
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			Commit = setting.Value
			const shortHashLen = 7
			if len(Commit) > shortHashLen {
				Commit = Commit[:shortHashLen]
			}
		case "vcs.time":
			Date = setting.Value
		}
	}
}

// Info holds structured version information for display and JSON output.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Get returns the current build information populated from ldflags variables
// and runtime constants.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}
