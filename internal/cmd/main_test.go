package cmd

import (
	"testing"

	"github.com/jedib0t/go-pretty/v6/text"
)

// go-pretty decides once, at init, whether to emit ANSI codes, from NO_COLOR
// and TERM, and a developer's NO_COLOR=1 would strip the codes a Colors row
// looks for. A run with Colors off never asks go-pretty for a color, so turning
// them on for the whole package changes no other row.
func TestMain(m *testing.M) {
	text.EnableColors()
	m.Run()
}
