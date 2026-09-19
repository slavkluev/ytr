package output_test

import (
	"testing"

	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/testutil"
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

func TestSetTTYOverridesDetection(t *testing.T) {
	testutil.ResetOutputFlags(t)

	output.SetTTY(true)
	if !output.IsTTY() {
		t.Error("IsTTY() = false after SetTTY(true)")
	}

	output.SetTTY(false)
	if output.IsTTY() {
		t.Error("IsTTY() = true after SetTTY(false)")
	}
}

func TestResetFlagsClearsTTYOverride(t *testing.T) {
	testutil.ResetOutputFlags(t)

	// Whatever stdout really is, the reset must hand detection back to it.
	detected := output.IsTTY()

	output.SetTTY(!detected)
	output.ResetFlags()

	if got := output.IsTTY(); got != detected {
		t.Errorf("IsTTY() = %v after ResetFlags, want the detected %v", got, detected)
	}
}

func TestColorsEnabledFollowsTheTTYOverride(t *testing.T) {
	testutil.ResetOutputFlags(t)
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("CLICOLOR", "")

	output.SetTTY(false)
	if output.ColorsEnabled() {
		t.Error("ColorsEnabled() = true off a TTY with no forcing variable")
	}

	output.SetTTY(true)
	if !output.ColorsEnabled() {
		t.Error("ColorsEnabled() = false on a TTY with no NO_COLOR")
	}
}
