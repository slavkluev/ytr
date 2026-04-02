// Package output provides TTY-aware rendering infrastructure for ytr CLI.
// It supports three output modes: table (human TTY), JSON (machine), and quiet (identifiers only).
package output

import (
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

const defaultTerminalWidth = 80

// IsTTY returns true if stdout is a terminal.
// It checks both standard terminals and Cygwin/MSYS2 terminals for Windows compatibility.
func IsTTY() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// TerminalWidth returns the current terminal width in columns.
// Returns 80 as the default when not connected to a terminal or on error.
func TerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd())) //nolint:gosec // fd conversion is safe for terminal operations
	if err != nil || width <= 0 {
		return defaultTerminalWidth
	}
	return width
}

// ColorsEnabled checks whether color output should be enabled.
// Precedence (highest to lowest):
//  1. NO_COLOR set and non-empty -> colors OFF (https://no-color.org/)
//  2. CLICOLOR_FORCE set and != "0" -> colors ON (even if not TTY)
//  3. CLICOLOR set -> colors ON only if value != "0" and is TTY
//  4. Default: colors ON if TTY, OFF if not
func ColorsEnabled() bool {
	if noColor, ok := os.LookupEnv("NO_COLOR"); ok && noColor != "" {
		return false
	}

	if force, ok := os.LookupEnv("CLICOLOR_FORCE"); ok && force != "" && force != "0" {
		return true
	}

	if cliColor, ok := os.LookupEnv("CLICOLOR"); ok {
		return cliColor != "0" && IsTTY()
	}

	return IsTTY()
}
