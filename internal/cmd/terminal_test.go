package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// terminalPackages are the packages that tell whether a stream is a terminal.
var terminalPackages = []string{"github.com/mattn/go-isatty", "golang.org/x/term"}

// terminalAuthDir holds auth login, the one command that prompts on a terminal.
// Every other command prints the same JSON whatever its streams are.
var terminalAuthDir = filepath.Join("..", "..", "internal", "cmd", "auth")

// terminalChecks returns a file:line entry for every way file can tell whether
// a stream is a terminal: an import of a terminal package, or ModeCharDevice,
// the file mode bit os and io/fs set on one.
func terminalChecks(fset *token.FileSet, file *ast.File) []string {
	var found []string
	report := func(pos token.Pos, what string) {
		p := fset.Position(pos)
		found = append(found, p.Filename+":"+strconv.Itoa(p.Line)+": "+what)
	}

	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err == nil && slices.Contains(terminalPackages, path) {
			report(imp.Pos(), "imports "+path)
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "ModeCharDevice" {
			report(sel.Pos(), "reads ModeCharDevice")
		}
		return true
	})

	return found
}

func TestOnlyAuthChecksForATerminal(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	parsed, inAuth := 0, 0

	for _, root := range []string{filepath.Join("..", "..", "internal"), filepath.Join("..", "..", "cmd")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			parsed++

			checks := terminalChecks(fset, file)
			if filepath.Dir(path) == terminalAuthDir {
				inAuth += len(checks)
				return nil
			}
			for _, check := range checks {
				t.Errorf("%s: only auth login may check for a terminal; every other command prints the same "+
					"JSON on a terminal and a pipe", check)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	if parsed == 0 {
		t.Fatal("parsed no source files, the walk is not reaching the tree")
	}
	if inAuth == 0 {
		t.Errorf("found no terminal check in %s either, so the check no longer recognizes one", terminalAuthDir)
	}
}

func TestTerminalCheckFindsImportsAndTheModeBit(t *testing.T) {
	t.Parallel()

	const src = `package probe

import (
	"io/fs"
	"os"

	tty "github.com/mattn/go-isatty"
	"golang.org/x/term"
)

func probe(f *os.File) bool {
	info, _ := f.Stat()
	_ = term.IsTerminal
	_ = tty.IsTerminal
	return info.Mode()&os.ModeCharDevice != 0 || info.Mode()&fs.ModeCharDevice != 0
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "probe.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got := terminalChecks(fset, file)

	want := []string{
		"probe.go:7: imports github.com/mattn/go-isatty",
		"probe.go:8: imports golang.org/x/term",
		"probe.go:15: reads ModeCharDevice",
		"probe.go:15: reads ModeCharDevice",
	}
	if !slices.Equal(got, want) {
		t.Errorf("checks = %q,\nwant %q", got, want)
	}
}
