package output

import (
	"os"
	"testing"
)

func env(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := vars[name]
		return value, ok
	}
}

func TestColorsEnabled_NOCOLORDisables(t *testing.T) {
	if colorsEnabled(env(map[string]string{"NO_COLOR": "1"}), true) {
		t.Error("colorsEnabled() = true, want false when NO_COLOR is set")
	}
}

func TestColorsEnabled_CLICOLORFORCEEnables(t *testing.T) {
	if !colorsEnabled(env(map[string]string{"NO_COLOR": "", "CLICOLOR_FORCE": "1"}), false) {
		t.Error("colorsEnabled() = false, want true when CLICOLOR_FORCE=1")
	}
}

func TestColorsEnabled_NOCOLOROverridesCLICOLORFORCE(t *testing.T) {
	if colorsEnabled(env(map[string]string{"NO_COLOR": "1", "CLICOLOR_FORCE": "1"}), true) {
		t.Error("colorsEnabled() = true, want false when NO_COLOR overrides CLICOLOR_FORCE")
	}
}

func TestColorsEnabled_CLICOLOROff(t *testing.T) {
	if colorsEnabled(env(map[string]string{"NO_COLOR": "", "CLICOLOR_FORCE": "", "CLICOLOR": "0"}), true) {
		t.Error("colorsEnabled() = true, want false when CLICOLOR=0")
	}
}

func TestColorsEnabled_CLICOLORFORCEZeroDoesNotForce(t *testing.T) {
	if colorsEnabled(env(map[string]string{"CLICOLOR_FORCE": "0"}), false) {
		t.Error("colorsEnabled() = true off a TTY, want false when CLICOLOR_FORCE=0")
	}
}

func TestColorsEnabled_CLICOLOROnDoesNotColorOffATTY(t *testing.T) {
	if colorsEnabled(env(map[string]string{"CLICOLOR": "1"}), false) {
		t.Error("colorsEnabled() = true off a TTY, want false when only CLICOLOR=1")
	}
}

func TestColorsEnabled_EmptyCLICOLORKeepsColorsOnATTY(t *testing.T) {
	if !colorsEnabled(env(map[string]string{"CLICOLOR": ""}), true) {
		t.Error("colorsEnabled() = false on a TTY, want true when CLICOLOR is empty")
	}
}

func TestColorsEnabledFollowsTheTTY(t *testing.T) {
	noVars := env(map[string]string{"NO_COLOR": "", "CLICOLOR_FORCE": "", "CLICOLOR": ""})

	if colorsEnabled(noVars, false) {
		t.Error("colorsEnabled() = true off a TTY with no forcing variable")
	}
	if !colorsEnabled(noVars, true) {
		t.Error("colorsEnabled() = false on a TTY with no NO_COLOR")
	}
}

func TestTerminalReportsARegularFileAsNoTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("CLICOLOR", "")

	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	opts := Terminal(f)

	if opts.TTY || opts.Colors {
		t.Errorf("Terminal(regular file) = TTY %v, colors %v, want both off", opts.TTY, opts.Colors)
	}
	if got := opts.TerminalWidth(); got != defaultTerminalWidth {
		t.Errorf("TerminalWidth() = %d, want the %d-column default", got, defaultTerminalWidth)
	}
	if opts.IsJSON() || opts.Quiet || opts.Debug {
		t.Errorf("Terminal(regular file) set a flag field: %+v", opts)
	}
}
