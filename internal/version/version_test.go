package version

import (
	"runtime"
	"testing"
)

// resetVars sets version variables to their defaults and returns a restore function.
func resetVars(t *testing.T) {
	t.Helper()
	origVersion := Version
	origCommit := Commit
	origDate := Date
	t.Cleanup(func() {
		Version = origVersion
		Commit = origCommit
		Date = origDate
	})
	Version = "dev"
	Commit = "none"
	Date = "unknown"
}

func TestInitFromBuildInfo_LdflagsSet(t *testing.T) {
	resetVars(t)

	// Simulate ldflags being set by GoReleaser
	Version = "1.0.0"
	Commit = "abc1234"
	Date = "2026-01-01"

	InitFromBuildInfo()

	// ldflags values must be preserved — fallback must not overwrite
	if Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", Version, "1.0.0")
	}
	if Commit != "abc1234" {
		t.Errorf("Commit = %q, want %q", Commit, "abc1234")
	}
	if Date != "2026-01-01" {
		t.Errorf("Date = %q, want %q", Date, "2026-01-01")
	}
}

func TestInitFromBuildInfo_DevVersion(t *testing.T) {
	resetVars(t)

	// When Version is "dev", InitFromBuildInfo should attempt to read build info.
	// In test binary context, debug.ReadBuildInfo returns info about the test binary.
	// The key assertion: function must not panic.
	InitFromBuildInfo()

	// After calling InitFromBuildInfo, Version may still be "dev" if the test binary
	// reports "(devel)" as Main.Version (typical for local test runs), or it may be
	// updated if the binary has proper VCS info. Either outcome is valid.
	if Version == "" {
		t.Error("Version must not be empty after InitFromBuildInfo")
	}
}

func TestGet_ReturnsInfo(t *testing.T) {
	info := Get()

	if info.Version == "" {
		t.Error("Version must not be empty")
	}
	if info.Commit == "" {
		t.Error("Commit must not be empty")
	}
	if info.GoVersion == "" {
		t.Error("GoVersion must not be empty")
	}
	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, runtime.Version())
	}
	if info.OS == "" {
		t.Error("OS must not be empty")
	}
	if info.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", info.OS, runtime.GOOS)
	}
	if info.Arch == "" {
		t.Error("Arch must not be empty")
	}
	if info.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", info.Arch, runtime.GOARCH)
	}
}
