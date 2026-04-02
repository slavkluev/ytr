package output_test

import (
	"testing"

	"github.com/slavkluev/ytr/internal/output"
)

func TestColorsEnabled_NOCOLORDisables(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("CLICOLOR", "")

	if output.ColorsEnabled() {
		t.Error("ColorsEnabled() = true, want false when NO_COLOR is set")
	}
}

func TestColorsEnabled_CLICOLORFORCEEnables(t *testing.T) {
	// Ensure NO_COLOR is not set by setting it to empty and unsetting
	// t.Setenv restores original value after test
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")

	if !output.ColorsEnabled() {
		t.Error("ColorsEnabled() = false, want true when CLICOLOR_FORCE=1")
	}
}

func TestColorsEnabled_NOCOLOROverridesCLICOLORFORCE(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "1")

	if output.ColorsEnabled() {
		t.Error("ColorsEnabled() = true, want false when NO_COLOR overrides CLICOLOR_FORCE")
	}
}

func TestColorsEnabled_CLICOLOROff(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("CLICOLOR", "0")

	if output.ColorsEnabled() {
		t.Error("ColorsEnabled() = true, want false when CLICOLOR=0")
	}
}
