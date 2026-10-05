package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/cmd/issue"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

func TestUnknownSubcommandUnderGroup(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"issue", "lst"})

	want := "Error: unknown command \"lst\" for \"ytr issue\"\nDid you mean: ytr issue list\n"
	if got.Stderr != want {
		t.Errorf("stderr = %q, want %q", got.Stderr, want)
	}
	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d", got.Code, ytrerrors.ExitUserError)
	}
	if got.Stdout != "" {
		t.Errorf("stdout = %q, want empty", got.Stdout)
	}
}

func TestUnknownSubcommandUnderGroupJSON(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"issue", "lst", "--json", "key"})

	want := `{"code":"user_error","message":"unknown command \"lst\" for \"ytr issue\"",` +
		`"suggestion":"Did you mean: ytr issue list"}` + "\n"
	if got.Stderr != want {
		t.Errorf("stderr = %q, want %q", got.Stderr, want)
	}
	if got.Stdout != "" {
		t.Errorf("stdout = %q, want empty", got.Stdout)
	}
}

func TestBareGroupNamesItsSubcommands(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"issue"})

	want := "Error: \"ytr issue\" needs a subcommand: changelog, create, list, transition, update, view\n" +
		"Run \"ytr issue --help\" for details.\n"
	if got.Stderr != want {
		t.Errorf("stderr = %q, want %q", got.Stderr, want)
	}
	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d", got.Code, ytrerrors.ExitUserError)
	}
	if got.Stdout != "" {
		t.Errorf("stdout = %q, want empty", got.Stdout)
	}
}

func TestBareRootNamesItsSubcommands(t *testing.T) {
	t.Parallel()

	got := runProbe(t, nil)

	// Pinned in full, not by Contains: the list has to be the one `ytr --help`
	// advertises, down to `help`, which cobra's IsAvailableCommand leaves out.
	want := "Error: \"ytr\" needs a subcommand: auth, bulk, checklist, comment, completion, component, " +
		"field, help, issue, issuetype, link, priority, queue, resolution, status, user, version, worklog\n" +
		"Run \"ytr --help\" for details.\n"
	if got.Stderr != want {
		t.Errorf("stderr = %q, want %q", got.Stderr, want)
	}
	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d", got.Code, ytrerrors.ExitUserError)
	}
	if got.Stdout != "" {
		t.Errorf("stdout = %q, want empty", got.Stdout)
	}
}

// TestBareRootListsEverythingHelpAdvertises keeps the failure's list and the
// help text from drifting apart; cobra's help template uses the same
// "available, or named help" rule.
func TestBareRootListsEverythingHelpAdvertises(t *testing.T) {
	t.Parallel()

	failure := runProbe(t, nil)
	help := runProbe(t, []string{"--help"})

	_, listed, found := strings.Cut(failure.Stderr, "needs a subcommand: ")
	if !found {
		t.Fatalf("stderr = %q, want it to list the subcommands", failure.Stderr)
	}
	listed, _, _ = strings.Cut(listed, "\n")

	for _, name := range strings.Split(listed, ", ") {
		if !strings.Contains(help.Stdout, "\n  "+name+" ") {
			t.Errorf("bare ytr lists %q but ytr --help does not advertise it", name)
		}
	}
}

func TestUnknownFlagBeforeJSONStillRendersJSON(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"issue", "list", "--nosuchflag", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr issue list --nosuchflag --json key", got.Stderr)
	if doc.Message != "unknown flag: --nosuchflag" {
		t.Errorf("message = %q, want %q", doc.Message, "unknown flag: --nosuchflag")
	}
	if doc.Suggestion != "Run \"ytr issue list --help\" for details." {
		t.Errorf("suggestion = %q, want the command's --help", doc.Suggestion)
	}
	if got.Stdout != "" {
		t.Errorf("stdout = %q, want empty", got.Stdout)
	}
}

// TestQuietIsAnUnknownFlag pins that --quiet is gone rather than ignored: a run
// that still passes it fails before any request, leaving stdout empty.
func TestQuietIsAnUnknownFlag(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"issue", "list", "--quiet"})

	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d", got.Code, ytrerrors.ExitUserError)
	}
	assertEmpty(t, "stdout", got.Stdout)
	if want := "Error: unknown flag: --quiet\n"; !strings.HasPrefix(got.Stderr, want) {
		t.Errorf("stderr = %q, want it to start with %q", got.Stderr, want)
	}
}

func TestUnknownFlagNamesTheClosestFlag(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"issue", "list", "--limitt", "5", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr issue list --limitt 5 --json key", got.Stderr)
	if doc.Suggestion != "Did you mean: --limit" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: --limit")
	}
}

func TestRootLevelTypoKeepsDidYouMean(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"isue", "list", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr isue list --json key", got.Stderr)
	if doc.Message != `unknown command "isue" for "ytr"` {
		t.Errorf("message = %q, want it to name the unknown command", doc.Message)
	}
	// The rest of what was typed is carried through, so the suggestion runs.
	if doc.Suggestion != "Did you mean: ytr issue list" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: ytr issue list")
	}
}

func TestUnknownHelpTopicFails(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"help", "isue"})

	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d (stderr: %s)", got.Code, ytrerrors.ExitUserError, got.Stderr)
	}
	if got.Stdout != "" {
		t.Errorf("stdout = %q, want empty", got.Stdout)
	}
	want := "Error: unknown command \"isue\" for \"ytr\"\nDid you mean: ytr issue\n"
	if got.Stderr != want {
		t.Errorf("stderr = %q, want %q", got.Stderr, want)
	}
}

func TestUnknownHelpSubtopicFails(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"help", "issue", "lst"})

	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d (stderr: %s)", got.Code, ytrerrors.ExitUserError, got.Stderr)
	}
	if !strings.Contains(got.Stderr, "Did you mean: ytr issue list") {
		t.Errorf("stderr = %q, want it to suggest ytr issue list", got.Stderr)
	}
}

func TestExplicitHelpStaysExitZero(t *testing.T) {
	t.Parallel()

	for _, argv := range [][]string{
		{"issue", "lst", "--help"},
		{"help", "issue"},
		{"help"},
		{"--help"},
		// An explicit help request is not command output, so --json does not
		// make it a JSON document and does not make it a failure either.
		{"issue", "list", "--json", "key", "--help"},
	} {
		label := "ytr " + strings.Join(argv, " ")
		got := runProbe(t, argv)

		if got.Code != ytrerrors.ExitSuccess {
			t.Errorf("%s: exit = %d, want %d (stderr: %s)",
				label, got.Code, ytrerrors.ExitSuccess, got.Stderr)
		}
		if got.Stderr != "" {
			t.Errorf("%s: stderr = %q, want empty", label, got.Stderr)
		}
		if !strings.Contains(got.Stdout, "Usage:") {
			t.Errorf("%s: stdout = %q, want the help text", label, got.Stdout)
		}
	}
}

func TestHelpTopicMatchesTheHelpFlag(t *testing.T) {
	t.Parallel()

	viaTopic := runProbe(t, []string{"help", "issue"})
	viaFlag := runProbe(t, []string{"issue", "--help"})

	if viaTopic.Stdout != viaFlag.Stdout {
		t.Errorf("ytr help issue and ytr issue --help disagree:\n%q\n%q", viaTopic.Stdout, viaFlag.Stdout)
	}
}

// TestSuccessPathIsUnchanged is the stdout-side mirror of assertRejected:
// exit 0, stderr empty, and stdout holding exactly one JSON document on one
// line, so a reader can take the whole stream as the answer.
func TestSuccessPathIsUnchanged(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"version", "--json", "version,commit"})

	if got.Code != ytrerrors.ExitSuccess {
		t.Errorf("exit = %d, want %d (stderr: %s)", got.Code, ytrerrors.ExitSuccess, got.Stderr)
	}
	if got.Stderr != "" {
		t.Errorf("stderr = %q, want empty", got.Stderr)
	}
	if strings.Count(got.Stdout, "\n") != 1 || !strings.HasSuffix(got.Stdout, "\n") {
		t.Errorf("stdout = %q, want exactly one JSON document", got.Stdout)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(got.Stdout), &doc); err != nil {
		t.Fatalf("stdout is not a JSON document (%v): %q", err, got.Stdout)
	}
	for _, field := range []string{"version", "commit"} {
		if _, ok := doc[field]; !ok {
			t.Errorf("stdout = %q, want a %s field", got.Stdout, field)
		}
	}
}

// TestJQStreamIsTheOnlyThingOnStdout pins the other success shape: --jq stays
// a jq stream with an implicit -r, one line per result and strings unquoted,
// and nothing else shares the stream with it.
func TestJQStreamIsTheOnlyThingOnStdout(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"version", "--jq", ".version, .os"})

	if got.Code != ytrerrors.ExitSuccess {
		t.Errorf("exit = %d, want %d (stderr: %s)", got.Code, ytrerrors.ExitSuccess, got.Stderr)
	}
	if got.Stderr != "" {
		t.Errorf("stderr = %q, want empty", got.Stderr)
	}

	lines := strings.Split(strings.TrimSuffix(got.Stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q, want one bare line per jq result", got.Stdout)
	}
	for _, line := range lines {
		if strings.HasPrefix(line, `"`) {
			t.Errorf("stdout line %q is quoted, want the raw string jq -r prints", line)
		}
	}
}

// TestJQFailureLeavesStdoutEmpty runs a filter that yields results and then
// fails. Nothing may reach stdout: a reader has no way to tell a truncated
// stream from a complete one.
func TestJQFailureLeavesStdoutEmpty(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"version", "--jq", ".version, (.os | .nosuchkey)"})

	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d (stdout: %q, stderr: %q)",
			got.Code, ytrerrors.ExitUserError, got.Stdout, got.Stderr)
	}
	if got.Stdout != "" {
		t.Errorf("stdout = %q, want empty when the filter fails", got.Stdout)
	}

	decodeOneJSONError(t, "ytr version --jq '.version, (.os | .nosuchkey)'", got.Stderr)
}

func TestEditDistanceIgnoresCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"list", "list", 0},
		{"LIST", "list", 0},
		{"lst", "list", 1},
		{"isue", "issue", 1},
		{"limitt", "limit", 1},
		{"abc", "", 3},
	}

	for _, c := range cases {
		if got := editDistance(c.a, c.b); got != c.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestUnknownShorthandFlagSuggestion(t *testing.T) {
	t.Parallel()

	// A one-letter typo only names a flag when it is a prefix of that flag's
	// name. The edit-distance rule is off for it, because every short flag name
	// is within distance two of any single letter.
	cases := []struct {
		shorthand string
		want      string
	}{
		{"-z", "Run \"ytr issue list --help\" for details."},
		{"-a", "Did you mean: --all"},
		{"-l", "Did you mean: --limit"},
	}

	for _, c := range cases {
		got := runProbe(t, []string{"issue", "list", c.shorthand, "--json", "key"})

		doc := decodeOneJSONError(t, "ytr issue list "+c.shorthand+" --json key", got.Stderr)
		if doc.Suggestion != c.want {
			t.Errorf("%s: suggestion = %q, want %q", c.shorthand, doc.Suggestion, c.want)
		}
	}
}

func TestUnknownFlagPrefixSuggestsTheFullName(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"issue", "list", "--al", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr issue list --al --json key", got.Stderr)
	if doc.Suggestion != "Did you mean: --all" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: --all")
	}
}

// TestExecuteWiring covers the exported Execute. os.Args, os.Stdin, os.Stdout
// and os.Stderr are wired up there and nowhere else, so nothing else in this
// suite notices when that wiring breaks.
func TestExecuteWiring(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", t.TempDir())

	realArgs, realIn, realOut, realErr := os.Args, os.Stdin, os.Stdout, os.Stderr
	t.Cleanup(func() { os.Args, os.Stdin, os.Stdout, os.Stderr = realArgs, realIn, realOut, realErr })

	cases := []struct {
		name          string
		argv          []string
		stdin         string
		wantCode      int
		wantOut       string
		wantErr       string
		wantErrPrefix string
	}{
		// version needs no credentials, so the good invocation stays offline.
		{
			name:     "good invocation",
			argv:     []string{"ytr", "version", "--json", "version"},
			wantCode: ytrerrors.ExitSuccess,
			wantOut:  `"version"`,
		},
		{
			name:     "bad invocation",
			argv:     []string{"ytr", "vershon", "--json", "version"},
			wantCode: ytrerrors.ExitUserError,
			wantErr:  `"code":"user_error"`,
		},
		{
			name:          "bad invocation without an output flag",
			argv:          []string{"ytr", "vershon"},
			wantCode:      ytrerrors.ExitUserError,
			wantErrPrefix: "Error: unknown command \"vershon\" for \"ytr\"\n",
		},
		// An empty stdin would fail with "no token provided" instead, and the
		// token is read before login sends any request.
		{
			name:     "stdin reaches the command",
			argv:     []string{"ytr", "auth", "login", "--org-id", "O"},
			stdin:    "\n",
			wantCode: ytrerrors.ExitUserError,
			wantErr:  "empty token from stdin",
		},
	}

	for _, c := range cases {
		stdout, stderr := redirectedStream(t, "stdout"), redirectedStream(t, "stderr")
		os.Args, os.Stdin, os.Stdout, os.Stderr = c.argv, streamHolding(t, c.stdin), stdout, stderr

		code := Execute()
		gotOut, gotErr := streamContents(t, stdout), streamContents(t, stderr)

		if code != c.wantCode {
			t.Errorf("%s: Execute() = %d, want %d (stdout %q, stderr %q)",
				c.name, code, c.wantCode, gotOut, gotErr)
		}
		if c.wantOut == "" && gotOut != "" {
			t.Errorf("%s: stdout = %q, want empty", c.name, gotOut)
		}
		if c.wantOut != "" && !strings.Contains(gotOut, c.wantOut) {
			t.Errorf("%s: stdout = %q, want it to contain %q", c.name, gotOut, c.wantOut)
		}
		if c.wantErr == "" && c.wantErrPrefix == "" && gotErr != "" {
			t.Errorf("%s: stderr = %q, want empty", c.name, gotErr)
		}
		if c.wantErr != "" && !strings.Contains(gotErr, c.wantErr) {
			t.Errorf("%s: stderr = %q, want it to contain %q", c.name, gotErr, c.wantErr)
		}
		if !strings.HasPrefix(gotErr, c.wantErrPrefix) {
			t.Errorf("%s: stderr = %q, want it to start with %q", c.name, gotErr, c.wantErrPrefix)
		}
	}
}

// redirectedStream returns a file that stands in for a standard stream.
func redirectedStream(t *testing.T, name string) *os.File {
	t.Helper()

	f, err := os.Create(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("creating %s stand-in: %v", name, err)
	}
	t.Cleanup(func() { _ = f.Close() })

	return f
}

// streamHolding returns a file that stands in for stdin and yields content.
func streamHolding(t *testing.T, content string) *os.File {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the stdin stand-in: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening the stdin stand-in: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	return f
}

// streamContents reads back what was written to a redirected stream.
func streamContents(t *testing.T, f *os.File) string {
	t.Helper()

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("reading %s: %v", f.Name(), err)
	}

	return string(data)
}

func TestSuggestionKeepsTheRemainingArguments(t *testing.T) {
	t.Parallel()

	// Dropping PROJ-1 would suggest a command that fails with
	// "accepts 1 arg(s), received 0" when run as printed.
	got := runProbe(t, []string{"issue", "vew", "PROJ-1", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr issue vew PROJ-1 --json key", got.Stderr)
	if doc.Suggestion != "Did you mean: ytr issue view PROJ-1" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: ytr issue view PROJ-1")
	}
}

func TestMistypedHelpCommandIsSuggested(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"hel", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr hel --json key", got.Stderr)
	if doc.Suggestion != "Did you mean: ytr help" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: ytr help")
	}
}

func TestHelpTopicCompletionOffersSubcommands(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"__complete", "help", ""})

	for _, want := range []string{"issue\t", "queue\t", "help\t"} {
		if !strings.Contains(got.Stdout, want) {
			t.Errorf("stdout = %q, want it to offer %q", got.Stdout, want)
		}
	}
}

func TestHelpTopicCompletionOffersNothingForAnUnknownTopic(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"__complete", "help", "nosuch", ""})

	// Only cobra's trailing directive line is expected.
	if offered := strings.TrimSpace(strings.TrimSuffix(got.Stdout, ":4\n")); offered != "" {
		t.Errorf("stdout = %q, want no completions for an unresolvable topic", got.Stdout)
	}
}

func TestJSONCompletionOffersTheFieldsSetOnACommandOutsideTheRunner(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"__complete", "issue", "changelog", "--json", ""})

	offered, _, _ := strings.Cut(got.Stdout, "\n:")
	if fields := strings.Split(offered, "\n"); !slices.Equal(fields, issue.IssueChangelogFields) {
		t.Errorf("completion offers %q, want issue changelog's fields %q", fields, issue.IssueChangelogFields)
	}
}

func TestDispatchOnlyCommandsDoNotAdvertiseTheirBareForm(t *testing.T) {
	t.Parallel()

	// The contract gives every group a RunE so cobra stops answering a bare
	// group with help and exit 0. Cobra prints a `ytr issue [flags]` usage line
	// for anything it considers runnable, and running that line exits 1.
	for _, argv := range [][]string{{"--help"}, {"issue", "--help"}, {"completion", "--help"}} {
		label := "ytr " + strings.Join(argv, " ")
		got := runProbe(t, argv)

		path := strings.Join(argv[:len(argv)-1], " ")
		bare := "\n  ytr " + strings.TrimSuffix(path+" ", " ") + " [flags]\n"
		if strings.Contains(got.Stdout, bare) {
			t.Errorf("%s: help advertises %q, which exits 1", label, strings.TrimSpace(bare))
		}
		if !strings.Contains(got.Stdout, "[command]") {
			t.Errorf("%s: help no longer shows the [command] usage line", label)
		}
	}
}

func TestLeafCommandsStillAdvertiseTheirFlags(t *testing.T) {
	t.Parallel()

	got := runProbe(t, []string{"issue", "list", "--help"})

	if !strings.Contains(got.Stdout, "\n  ytr issue list [flags]\n") {
		t.Errorf("stdout = %q, want the usage line for a command that really runs", got.Stdout)
	}
}
