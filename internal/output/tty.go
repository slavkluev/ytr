// Package output provides TTY-aware rendering infrastructure for ytr CLI.
// It supports three output modes: table (human TTY), JSON (machine), and quiet (identifiers only).
package output

import (
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

const defaultTerminalWidth = 80

// Terminal returns the options a run writing to f starts from: whether f is a
// terminal, its width, and whether colors are on by the environment's rules.
// The flag fields are left zero for the root command to fill.
func Terminal(f *os.File) Options {
	_, tty := TerminalFile(f)

	width, _, err := term.GetSize(int(f.Fd())) //nolint:gosec // fd conversion is safe for terminal operations
	if err != nil {
		width = 0
	}

	return Options{
		TTY:    tty,
		Colors: colorsEnabled(os.LookupEnv, tty),
		Width:  width,
	}
}

// TerminalFile returns stream as a file and whether that file is open on a
// terminal. A stream that is not an *os.File, such as a test's buffer, is never
// a terminal.
func TerminalFile(stream any) (*os.File, bool) {
	f, ok := stream.(*os.File)
	if !ok {
		return nil, false
	}

	fd := f.Fd()

	return f, isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// TerminalWidth returns the width a table may fill, falling back to 80 columns
// when the width is unknown.
func (o *Options) TerminalWidth() int {
	if o.Width <= 0 {
		return defaultTerminalWidth
	}
	return o.Width
}

// colorsEnabled checks whether color output should be enabled.
// Precedence (highest to lowest):
//  1. NO_COLOR set and non-empty -> colors OFF (https://no-color.org/)
//  2. CLICOLOR_FORCE set and != "0" -> colors ON (even if not TTY)
//  3. CLICOLOR set -> colors ON only if value != "0" and is TTY
//  4. Default: colors ON if TTY, OFF if not
func colorsEnabled(lookup func(string) (string, bool), tty bool) bool {
	if noColor, ok := lookup("NO_COLOR"); ok && noColor != "" {
		return false
	}

	if force, ok := lookup("CLICOLOR_FORCE"); ok && force != "" && force != "0" {
		return true
	}

	if cliColor, ok := lookup("CLICOLOR"); ok {
		return cliColor != "0" && tty
	}

	return tty
}
