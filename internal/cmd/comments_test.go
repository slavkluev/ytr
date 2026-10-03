package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// restatingDocLine is the opening of a doc comment that only repeats an
// unexported name. An exported name starts with a capital, so its Go-convention
// doc comment never matches.
var restatingDocLine = regexp.MustCompile(`^(new|run|render)\w* (creates|executes|handles|renders|returns)\b`)

var testabilityPhrases = []string{
	"for testability",
	"replaceable for testing",
	"tests override this",
	"for testing purposes",
	"used in tests",
	"tests use this",
	"only tests call",
	"enables testing",
}

func docComment(n ast.Node) *ast.CommentGroup {
	switch node := n.(type) {
	case *ast.FuncDecl:
		return node.Doc
	case *ast.GenDecl:
		return node.Doc
	case *ast.TypeSpec:
		return node.Doc
	case *ast.ValueSpec:
		return node.Doc
	case *ast.Field:
		return node.Doc
	default:
		return nil
	}
}

// commentViolations returns a file:line entry for every doc comment in file that
// opens by restating its name and for every comment that says the code is
// shaped for tests.
func commentViolations(fset *token.FileSet, file *ast.File) []string {
	var found []string
	report := func(pos token.Pos, what string) {
		p := fset.Position(pos)
		found = append(found, p.Filename+":"+strconv.Itoa(p.Line)+": "+what)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		if doc := docComment(n); doc != nil {
			firstLine, _, _ := strings.Cut(doc.Text(), "\n")
			if restatingDocLine.MatchString(firstLine) {
				report(doc.Pos(), "doc comment restates its name: "+firstLine)
			}
		}
		return true
	})

	for _, group := range file.Comments {
		for _, c := range group.List {
			lower := strings.ToLower(c.Text)
			for _, phrase := range testabilityPhrases {
				if strings.Contains(lower, phrase) {
					report(c.Pos(), "comment explains test plumbing: "+c.Text)
				}
			}
		}
	}

	return found
}

func TestSourceCommentsStateOnlyAWhy(t *testing.T) {
	fset := token.NewFileSet()
	parsed := 0

	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			parsed++

			for _, v := range commentViolations(fset, file) {
				t.Error(v)
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
}

func TestCommentCheckRejectsRestatingAndTestabilityShapes(t *testing.T) {
	const src = `package probe

// newFoo creates a foo.
func newFoo() {}

// NewCmd creates the issue command.
func NewCmd() {}

// newBar returns a bar.
var newBar = 1

var (
	// runBaz executes the baz.
	runBaz = 2
)

type (
	// renderer handles output.
	renderer struct {
		// renderMode renders the mode.
		renderMode int
	}
)

func helper() {
	// Kept for testability.
	// Replaceable for testing.
	// Tests override this.
	// Exported for testing purposes.
	// Used in tests.
	// Tests use this to capture output.
	// Only tests call it.
	// This variant enables testing.
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "probe.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got := commentViolations(fset, file)

	// One restating doc per declaration kind, prefix and verb, then one line
	// per testability phrase. Line 6, the exported NewCmd doc, must not appear.
	wantLines := []int{3, 9, 13, 18, 20, 26, 27, 28, 29, 30, 31, 32, 33}
	if len(got) != len(wantLines) {
		t.Fatalf("got %d violations, want %d: %v", len(got), len(wantLines), got)
	}
	for i, line := range wantLines {
		prefix := "probe.go:" + strconv.Itoa(line) + ": "
		if !strings.HasPrefix(got[i], prefix) {
			t.Errorf("violation %d = %q, want it at %s", i, got[i], prefix)
		}
	}
}
