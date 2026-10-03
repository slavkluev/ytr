package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/issue"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
)

// The names no command and no flag answers to. Probes are built from them, so
// nothing here can reach RunE and therefore nothing can reach the API.
const (
	unknownCommandProbe = "ytr-no-such-command"
	unknownFlagProbe    = "--ytr-no-such-flag"
)

// maxProbeArgCount is how far the suite looks for a positional argument count a
// command rejects. The widest validator in the tree takes two arguments, so
// three also covers "one too many".
const maxProbeArgCount = 3

// probeResult is what one probe produced: the exit code the binary would return
// and the bytes each stream received.
type probeResult struct {
	code   int
	stdout string
	stderr string
}

// runProbe runs argv through the same path as the binary: a tree built for this
// run, cobra, then the one error renderer.
func runProbe(t *testing.T, argv []string) probeResult {
	t.Helper()
	// No probe is supposed to reach RunE, so none should ever need credentials.
	// Taking them away means a probe that did reach RunE fails on auth, exit 3,
	// instead of sending a request to the real Tracker.
	t.Setenv("YTR_CONFIG_DIR", t.TempDir())
	t.Setenv("YTR_TOKEN", "")
	t.Setenv("YTR_ORG_ID", "")
	t.Setenv("YTR_ORG_TYPE", "")

	var out, errOut bytes.Buffer
	// Zero options put the probe off a TTY, as an agent reads ytr, and keep
	// ANSI codes out of the bytes these tests compare.
	code := execute(t.Context(), output.Options{}, argv, &out, &errOut)

	return probeResult{code: code, stdout: out.String(), stderr: errOut.String()}
}

// errorDocument is the JSON error shape a failed invocation must produce.
type errorDocument struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// assertRejected runs argv with a --json field selection appended after whatever
// it gets wrong, and fails unless the contract held: exit 1, nothing on stdout,
// and exactly one JSON error document on stderr. Appending --json last is what
// proves it is honoured even when the mistake stopped flag parsing first.
func assertRejected(t *testing.T, argv []string) errorDocument {
	t.Helper()

	probe := slices.Concat(argv, []string{"--json", "key"})
	label := "ytr " + strings.Join(probe, " ")
	got := runProbe(t, probe)

	if got.code != ytrerrors.ExitUserError {
		t.Errorf("%s: exit = %d, want %d (stderr: %s)", label, got.code, ytrerrors.ExitUserError, got.stderr)
	}
	if got.stdout != "" {
		t.Errorf("%s: stdout = %q, want empty", label, got.stdout)
	}

	return decodeOneJSONError(t, label, got.stderr)
}

// decodeOneJSONError fails unless stderr holds exactly one JSON error document
// carrying a code and a message.
func decodeOneJSONError(t *testing.T, label, stderr string) errorDocument {
	t.Helper()

	var doc errorDocument

	trimmed := strings.TrimSuffix(stderr, "\n")
	if trimmed == "" || strings.Contains(trimmed, "\n") {
		t.Errorf("%s: stderr = %q, want exactly one JSON document", label, stderr)
		return doc
	}

	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		t.Errorf("%s: stderr is not a JSON document (%v): %q", label, err, stderr)
		return doc
	}

	if doc.Code != ytrerrors.CodeUserError {
		t.Errorf("%s: code = %q, want %q", label, doc.Code, ytrerrors.CodeUserError)
	}
	if doc.Message == "" {
		t.Errorf("%s: message is empty", label)
	}

	return doc
}

// withoutDebugLines drops the diagnostics --debug writes to stderr, leaving
// what a reader has to find there on its own: the single error document.
func withoutDebugLines(stderr string) string {
	var kept []string
	for _, line := range strings.Split(stderr, "\n") {
		if !strings.HasPrefix(line, "[debug] ") {
			kept = append(kept, line)
		}
	}

	return strings.Join(kept, "\n")
}

// walkCommands calls visit for cmd and every command below it.
func walkCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, sub := range cmd.Commands() {
		walkCommands(sub, visit)
	}
}

// argPath returns the arguments that select cmd, the binary name excluded.
func argPath(cmd *cobra.Command) []string {
	return strings.Fields(cmd.CommandPath())[1:]
}

// commandPaths returns the argument path of every command in a freshly built
// tree that want accepts. The probes come from the tree, so a command added
// later is covered without this file being edited.
func commandPaths(t *testing.T, want func(*cobra.Command) bool) [][]string {
	t.Helper()

	var paths [][]string
	walkCommands(newRootCmd(&output.Options{}), func(cmd *cobra.Command) {
		if want(cmd) {
			paths = append(paths, argPath(cmd))
		}
	})

	return paths
}

// hasSubCommands and acceptAnyCommand select what commandPaths returns.
func hasSubCommands(cmd *cobra.Command) bool { return cmd.HasSubCommands() }
func acceptAnyCommand(*cobra.Command) bool   { return true }

// pathSet indexes argument paths by their joined form.
func pathSet(paths [][]string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, path := range paths {
		set[strings.Join(path, " ")] = true
	}

	return set
}

// placeholderArgs returns count positional arguments that are neither a flag nor
// the name of any command.
func placeholderArgs(count int) []string {
	args := make([]string, 0, count)
	for i := range count {
		args = append(args, "ytr-probe-"+strconv.Itoa(i))
	}

	return args
}

// rejectedArgCounts returns every positional argument count up to
// maxProbeArgCount that cmd's declared validator rejects, so a leaf is probed at
// both ends of its accepted range. Returning only the first rejected count would
// mean too FEW arguments every time: a validator that wants one rejects zero
// before anything else.
//
// The result is empty for a validator that rejects nothing -- cobra.ArbitraryArgs
// -- which is a deliberate declaration rather than an omission, so such a leaf
// gets no count probe and is covered by the unknown-flag probe.
func rejectedArgCounts(cmd *cobra.Command) []int {
	var counts []int
	for count := 0; count <= maxProbeArgCount; count++ {
		if cmd.Args(cmd, placeholderArgs(count)) != nil {
			counts = append(counts, count)
		}
	}

	return counts
}

// argCountProbe is one leaf and a number of positional arguments it rejects.
type argCountProbe struct {
	path  []string
	count int
}

// argCountProbes derives a probe per leaf per rejected count, from the leaf's own
// Args validator.
func argCountProbes(t *testing.T) []argCountProbe {
	t.Helper()

	var probes []argCountProbe
	walkCommands(newRootCmd(&output.Options{}), func(cmd *cobra.Command) {
		if cmd.HasSubCommands() || cmd.Args == nil {
			return
		}
		for _, count := range rejectedArgCounts(cmd) {
			probes = append(probes, argCountProbe{path: argPath(cmd), count: count})
		}
	})

	return probes
}

// rejectionProbe is one bad invocation, kept as a command path and the mistake
// appended to it so --json can be placed on either side of the mistake.
type rejectionProbe struct {
	path    []string
	mistake []string
}

// derivedRejections returns every bad invocation the suite derives from the
// tree: a group named without a subcommand, a group given one that does not
// exist, any command given an unknown flag, and any leaf given a positional
// argument count its own validator rejects.
func derivedRejections(t *testing.T) []rejectionProbe {
	t.Helper()

	var probes []rejectionProbe

	for _, path := range commandPaths(t, hasSubCommands) {
		probes = append(probes,
			rejectionProbe{path: path},
			rejectionProbe{path: path, mistake: []string{unknownCommandProbe}},
		)
	}

	for _, path := range commandPaths(t, acceptAnyCommand) {
		probes = append(probes, rejectionProbe{
			path: path, mistake: []string{unknownFlagProbe},
		})
	}

	for _, probe := range argCountProbes(t) {
		probes = append(probes, rejectionProbe{
			path: probe.path, mistake: placeholderArgs(probe.count),
		})
	}

	return probes
}

// debugForms returns the two ways to run probe under --debug: --json read
// before the mistake, and --json reachable only by re-reading the raw
// arguments because the mistake stopped flag parsing. Which one it was used to
// decide whether the error document landed on stdout or on stderr.
func debugForms(probe rejectionProbe) [][]string {
	return [][]string{
		slices.Concat(probe.path, []string{"--debug", "--json", "key"}, probe.mistake),
		slices.Concat(probe.path, []string{"--debug"}, probe.mistake, []string{"--json", "key"}),
	}
}

// TestContractDebugKeepsStdoutEmpty runs every derived bad invocation under
// --debug in both --json positions. The probes come from the tree, so a
// command added later is covered without this file being edited.
func TestContractDebugKeepsStdoutEmpty(t *testing.T) {
	probes := derivedRejections(t)
	if len(probes) == 0 {
		t.Fatal("no bad invocation was derived, the walk is not reaching the tree")
	}

	for _, probe := range probes {
		for _, argv := range debugForms(probe) {
			label := "ytr " + strings.Join(argv, " ")
			got := runProbe(t, argv)

			if got.code != ytrerrors.ExitUserError {
				t.Errorf("%s: exit = %d, want %d (stderr: %s)",
					label, got.code, ytrerrors.ExitUserError, got.stderr)
			}
			if got.stdout != "" {
				t.Errorf("%s: stdout = %q, want empty", label, got.stdout)
			}

			decodeOneJSONError(t, label, withoutDebugLines(got.stderr))
		}
	}
}

func TestContractEveryLeafDeclaresArgs(t *testing.T) {
	walkCommands(newRootCmd(&output.Options{}), func(cmd *cobra.Command) {
		if cmd.HasSubCommands() || cmd.Args != nil {
			return
		}
		t.Errorf("%q declares no Args, so cobra accepts and silently ignores any positional "+
			"argument; declare cobra.NoArgs, cobra.ExactArgs(n) or cobra.ArbitraryArgs",
			cmd.CommandPath())
	})
}

func TestContractGroupsRejectUnknownSubcommand(t *testing.T) {
	paths := commandPaths(t, hasSubCommands)
	if len(paths) == 0 {
		t.Fatal("no command with subcommands found, the walk is not reaching the tree")
	}

	for _, path := range paths {
		label := "ytr " + strings.Join(path, " ")
		doc := assertRejected(t, slices.Concat(path, []string{unknownCommandProbe}))

		if !strings.Contains(doc.Message, unknownCommandProbe) {
			t.Errorf("%s: message %q does not name the rejected argument", label, doc.Message)
		}
		if doc.Suggestion == "" {
			t.Errorf("%s: suggestion is empty", label)
		}
	}
}

func TestContractGroupsRejectMissingSubcommand(t *testing.T) {
	paths := commandPaths(t, hasSubCommands)
	if len(paths) == 0 {
		t.Fatal("no command with subcommands found, the walk is not reaching the tree")
	}

	for _, path := range paths {
		label := "ytr " + strings.Join(path, " ")
		doc := assertRejected(t, path)

		if !strings.Contains(doc.Message, "needs a subcommand") {
			t.Errorf("%s: message %q does not say a subcommand is missing", label, doc.Message)
		}
		if !strings.Contains(doc.Suggestion, "--help") {
			t.Errorf("%s: suggestion %q does not point at --help", label, doc.Suggestion)
		}
	}
}

func TestContractEveryCommandRejectsUnknownFlag(t *testing.T) {
	paths := commandPaths(t, acceptAnyCommand)
	if len(paths) == 0 {
		t.Fatal("no command found, the walk is not reaching the tree")
	}

	for _, path := range paths {
		label := "ytr " + strings.Join(path, " ")
		doc := assertRejected(t, slices.Concat(path, []string{unknownFlagProbe}))

		if !strings.Contains(doc.Message, unknownFlagProbe) {
			t.Errorf("%s: message %q does not name the rejected flag", label, doc.Message)
		}
		if doc.Suggestion == "" {
			t.Errorf("%s: suggestion is empty", label)
		}
	}
}

func TestContractEveryLeafRejectsBadArgCount(t *testing.T) {
	probes := argCountProbes(t)
	if len(probes) == 0 {
		t.Fatal("no leaf declares an argument count it rejects, the walk is not reaching the tree")
	}

	for _, probe := range probes {
		label := "ytr " + strings.Join(probe.path, " ")
		doc := assertRejected(t, slices.Concat(probe.path, placeholderArgs(probe.count)))

		if doc.Suggestion == "" {
			t.Errorf("%s with %d argument(s): suggestion is empty", label, probe.count)
		}
	}
}

// issueKeyArgs returns the fewest positional arguments cmd accepts, each shaped
// like an issue key, so an argument validator lets the probe reach flag checks.
func issueKeyArgs(t *testing.T, cmd *cobra.Command) []string {
	t.Helper()

	for count := range maxProbeArgCount + 1 {
		args := slices.Repeat([]string{"PROJ-1"}, count)
		if cmd.Args(cmd, args) == nil {
			return args
		}
	}

	t.Fatalf("%q accepts no argument count up to %d", cmd.CommandPath(), maxProbeArgCount)
	return nil
}

// TestEveryLeafRejectsAllWithCursor finds every leaf that offers both --all and
// --cursor and checks that it refuses the pair instead of letting --all win
// silently. runProbe clears credentials, so a leaf that checked auth first would
// exit 3 here rather than 1.
func TestEveryLeafRejectsAllWithCursor(t *testing.T) {
	var probed []string
	walkCommands(newRootCmd(&output.Options{}), func(cmd *cobra.Command) {
		if cmd.HasSubCommands() || cmd.Flags().Lookup("all") == nil || cmd.Flags().Lookup("cursor") == nil {
			return
		}
		probed = append(probed, cmd.CommandPath())

		argv := slices.Concat(argPath(cmd), issueKeyArgs(t, cmd), []string{"--all", "--cursor", "2"})
		label := "ytr " + strings.Join(argv, " ")
		got := runProbe(t, argv)

		if got.code != ytrerrors.ExitUserError {
			t.Errorf("%s: exit = %d, want %d (stderr: %s)", label, got.code, ytrerrors.ExitUserError, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("%s: stdout = %q, want empty", label, got.stdout)
		}
		if !strings.Contains(got.stderr, "cannot combine --all with --cursor") {
			t.Errorf("%s: stderr = %q, want it to name the conflict", label, got.stderr)
		}
	})

	for _, known := range []string{"ytr issue list", "ytr issue changelog", "ytr queue list", "ytr user list"} {
		if !slices.Contains(probed, known) {
			t.Errorf("%q was not probed; the walk found only %v", known, probed)
		}
	}
}

// TestContractProbeSetCoversTheTree checks the derived probe set against the tree
// itself: it reads each command's Args validator directly instead of going
// through rejectedArgCounts, so it disagrees when the derivation is what broke --
// a leaf the walk skipped, a wrong argument path, or a flattening that kept only
// one of the counts a validator rejects.
func TestContractProbeSetCoversTheTree(t *testing.T) {
	flagProbed := pathSet(commandPaths(t, acceptAnyCommand))
	subcommandProbed := pathSet(commandPaths(t, hasSubCommands))

	countProbed := make(map[string][]int)
	for _, probe := range argCountProbes(t) {
		key := strings.Join(probe.path, " ")
		countProbed[key] = append(countProbed[key], probe.count)
	}

	walkCommands(newRootCmd(&output.Options{}), func(cmd *cobra.Command) {
		path := strings.Join(argPath(cmd), " ")

		if !flagProbed[path] {
			t.Errorf("%q gets no unknown-flag probe", cmd.CommandPath())
		}

		if cmd.HasSubCommands() {
			if !subcommandProbed[path] {
				t.Errorf("%q has subcommands but gets no subcommand probe", cmd.CommandPath())
			}
			return
		}

		if cmd.Args == nil {
			return // TestContractEveryLeafDeclaresArgs reports this one.
		}

		var want []int
		for count := range maxProbeArgCount + 1 {
			if cmd.Args(cmd, placeholderArgs(count)) != nil {
				want = append(want, count)
			}
		}

		if !slices.Equal(countProbed[path], want) {
			t.Errorf("%q is probed with argument counts %v, want %v",
				cmd.CommandPath(), countProbed[path], want)
		}
	})
}

func TestUnknownSubcommandUnderGroup(t *testing.T) {
	got := runProbe(t, []string{"issue", "lst"})

	want := "Error: unknown command \"lst\" for \"ytr issue\"\nDid you mean: ytr issue list\n"
	if got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
	if got.code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d", got.code, ytrerrors.ExitUserError)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
}

func TestUnknownSubcommandUnderGroupJSON(t *testing.T) {
	got := runProbe(t, []string{"issue", "lst", "--json", "key"})

	want := `{"code":"user_error","message":"unknown command \"lst\" for \"ytr issue\"",` +
		`"suggestion":"Did you mean: ytr issue list"}` + "\n"
	if got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
}

func TestBareGroupNamesItsSubcommands(t *testing.T) {
	got := runProbe(t, []string{"issue"})

	want := "Error: \"ytr issue\" needs a subcommand: changelog, create, list, transition, update, view\n" +
		"Run \"ytr issue --help\" for details.\n"
	if got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
	if got.code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d", got.code, ytrerrors.ExitUserError)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
}

func TestBareRootNamesItsSubcommands(t *testing.T) {
	got := runProbe(t, nil)

	// Pinned in full, not by Contains: the list has to be the one `ytr --help`
	// advertises, down to `help`, which cobra's IsAvailableCommand leaves out.
	want := "Error: \"ytr\" needs a subcommand: auth, bulk, checklist, comment, completion, component, " +
		"field, help, issue, issuetype, link, priority, queue, resolution, status, user, version, worklog\n" +
		"Run \"ytr --help\" for details.\n"
	if got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
	if got.code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d", got.code, ytrerrors.ExitUserError)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
}

// TestBareRootListsEverythingHelpAdvertises keeps the failure's list and the
// help text from drifting apart; cobra's help template uses the same
// "available, or named help" rule.
func TestBareRootListsEverythingHelpAdvertises(t *testing.T) {
	failure := runProbe(t, nil)
	help := runProbe(t, []string{"--help"})

	_, listed, found := strings.Cut(failure.stderr, "needs a subcommand: ")
	if !found {
		t.Fatalf("stderr = %q, want it to list the subcommands", failure.stderr)
	}
	listed, _, _ = strings.Cut(listed, "\n")

	for _, name := range strings.Split(listed, ", ") {
		if !strings.Contains(help.stdout, "\n  "+name+" ") {
			t.Errorf("bare ytr lists %q but ytr --help does not advertise it", name)
		}
	}
}

func TestUnknownFlagBeforeJSONStillRendersJSON(t *testing.T) {
	got := runProbe(t, []string{"issue", "list", "--nosuchflag", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr issue list --nosuchflag --json key", got.stderr)
	if doc.Message != "unknown flag: --nosuchflag" {
		t.Errorf("message = %q, want %q", doc.Message, "unknown flag: --nosuchflag")
	}
	if doc.Suggestion != "Run \"ytr issue list --help\" for details." {
		t.Errorf("suggestion = %q, want the command's --help", doc.Suggestion)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
}

func TestUnknownFlagNamesTheClosestFlag(t *testing.T) {
	got := runProbe(t, []string{"issue", "list", "--limitt", "5", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr issue list --limitt 5 --json key", got.stderr)
	if doc.Suggestion != "Did you mean: --limit" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: --limit")
	}
}

func TestRootLevelTypoKeepsDidYouMean(t *testing.T) {
	got := runProbe(t, []string{"isue", "list", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr isue list --json key", got.stderr)
	if doc.Message != `unknown command "isue" for "ytr"` {
		t.Errorf("message = %q, want it to name the unknown command", doc.Message)
	}
	// The rest of what was typed is carried through, so the suggestion runs.
	if doc.Suggestion != "Did you mean: ytr issue list" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: ytr issue list")
	}
}

func TestStrayPositionalOnFlagsOnlyLeaf(t *testing.T) {
	for _, path := range [][]string{
		{"issue", "list"},
		{"issue", "create"},
		{"queue", "list"},
		{"user", "list"},
	} {
		label := "ytr " + strings.Join(path, " ")
		got := runProbe(t, slices.Concat(path, []string{"APP"}))

		if got.code != ytrerrors.ExitUserError {
			t.Errorf("%s APP: exit = %d, want %d (stderr: %s)",
				label, got.code, ytrerrors.ExitUserError, got.stderr)
		}
		if !strings.Contains(got.stderr, "APP") {
			t.Errorf("%s APP: stderr = %q, does not name the rejected argument", label, got.stderr)
		}
		if got.stdout != "" {
			t.Errorf("%s APP: stdout = %q, want empty", label, got.stdout)
		}
	}
}

func TestUnknownHelpTopicFails(t *testing.T) {
	got := runProbe(t, []string{"help", "isue"})

	if got.code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d (stderr: %s)", got.code, ytrerrors.ExitUserError, got.stderr)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
	want := "Error: unknown command \"isue\" for \"ytr\"\nDid you mean: ytr issue\n"
	if got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
}

func TestUnknownHelpSubtopicFails(t *testing.T) {
	got := runProbe(t, []string{"help", "issue", "lst"})

	if got.code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d (stderr: %s)", got.code, ytrerrors.ExitUserError, got.stderr)
	}
	if !strings.Contains(got.stderr, "Did you mean: ytr issue list") {
		t.Errorf("stderr = %q, want it to suggest ytr issue list", got.stderr)
	}
}

func TestExplicitHelpStaysExitZero(t *testing.T) {
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

		if got.code != ytrerrors.ExitSuccess {
			t.Errorf("%s: exit = %d, want %d (stderr: %s)",
				label, got.code, ytrerrors.ExitSuccess, got.stderr)
		}
		if got.stderr != "" {
			t.Errorf("%s: stderr = %q, want empty", label, got.stderr)
		}
		if !strings.Contains(got.stdout, "Usage:") {
			t.Errorf("%s: stdout = %q, want the help text", label, got.stdout)
		}
	}
}

func TestHelpTopicMatchesTheHelpFlag(t *testing.T) {
	viaTopic := runProbe(t, []string{"help", "issue"})
	viaFlag := runProbe(t, []string{"issue", "--help"})

	if viaTopic.stdout != viaFlag.stdout {
		t.Errorf("ytr help issue and ytr issue --help disagree:\n%q\n%q", viaTopic.stdout, viaFlag.stdout)
	}
}

// TestSuccessPathIsUnchanged is the stdout-side mirror of assertRejected:
// exit 0, stderr empty, and stdout holding exactly one JSON document -- one
// line off a terminal, so a reader can take the whole stream as the answer.
func TestSuccessPathIsUnchanged(t *testing.T) {
	got := runProbe(t, []string{"version", "--json", "version,commit"})

	if got.code != ytrerrors.ExitSuccess {
		t.Errorf("exit = %d, want %d (stderr: %s)", got.code, ytrerrors.ExitSuccess, got.stderr)
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}
	if strings.Count(got.stdout, "\n") != 1 || !strings.HasSuffix(got.stdout, "\n") {
		t.Errorf("stdout = %q, want exactly one JSON document", got.stdout)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("stdout is not a JSON document (%v): %q", err, got.stdout)
	}
	for _, field := range []string{"version", "commit"} {
		if _, ok := doc[field]; !ok {
			t.Errorf("stdout = %q, want a %s field", got.stdout, field)
		}
	}
}

// TestJQStreamIsTheOnlyThingOnStdout pins the other success shape: --jq stays
// a jq stream with an implicit -r, one line per result and strings unquoted,
// and nothing else shares the stream with it.
func TestJQStreamIsTheOnlyThingOnStdout(t *testing.T) {
	got := runProbe(t, []string{"version", "--jq", ".version, .os"})

	if got.code != ytrerrors.ExitSuccess {
		t.Errorf("exit = %d, want %d (stderr: %s)", got.code, ytrerrors.ExitSuccess, got.stderr)
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}

	lines := strings.Split(strings.TrimSuffix(got.stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q, want one bare line per jq result", got.stdout)
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
	got := runProbe(t, []string{"version", "--jq", ".version, (.os | .nosuchkey)"})

	if got.code != ytrerrors.ExitUserError {
		t.Errorf("exit = %d, want %d (stdout: %q, stderr: %q)",
			got.code, ytrerrors.ExitUserError, got.stdout, got.stderr)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty when the filter fails", got.stdout)
	}

	decodeOneJSONError(t, "ytr version --jq '.version, (.os | .nosuchkey)'", got.stderr)
}

func TestEditDistanceIgnoresCase(t *testing.T) {
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

		doc := decodeOneJSONError(t, "ytr issue list "+c.shorthand+" --json key", got.stderr)
		if doc.Suggestion != c.want {
			t.Errorf("%s: suggestion = %q, want %q", c.shorthand, doc.Suggestion, c.want)
		}
	}
}

func TestUnknownFlagPrefixSuggestsTheFullName(t *testing.T) {
	got := runProbe(t, []string{"issue", "list", "--al", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr issue list --al --json key", got.stderr)
	if doc.Suggestion != "Did you mean: --all" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: --all")
	}
}

// TestExecuteWiring covers the exported Execute. os.Args, os.Stdout and
// os.Stderr are wired up there and nowhere else, so nothing else in this suite
// notices when that wiring breaks.
func TestExecuteWiring(t *testing.T) {
	t.Setenv("YTR_CONFIG_DIR", t.TempDir())
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")

	realArgs, realOut, realErr := os.Args, os.Stdout, os.Stderr
	t.Cleanup(func() { os.Args, os.Stdout, os.Stderr = realArgs, realOut, realErr })

	cases := []struct {
		name          string
		argv          []string
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
			name:          "bad invocation in human mode takes colors from the environment",
			argv:          []string{"ytr", "vershon"},
			wantCode:      ytrerrors.ExitUserError,
			wantErrPrefix: "\x1b[1;31mError\x1b[0m",
		},
	}

	for _, c := range cases {
		stdout, stderr := redirectedStream(t, "stdout"), redirectedStream(t, "stderr")
		os.Args, os.Stdout, os.Stderr = c.argv, stdout, stderr

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
	// Dropping PROJ-1 would suggest a command that fails with
	// "accepts 1 arg(s), received 0" when run as printed.
	got := runProbe(t, []string{"issue", "vew", "PROJ-1", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr issue vew PROJ-1 --json key", got.stderr)
	if doc.Suggestion != "Did you mean: ytr issue view PROJ-1" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: ytr issue view PROJ-1")
	}
}

func TestMistypedHelpCommandIsSuggested(t *testing.T) {
	got := runProbe(t, []string{"hel", "--json", "key"})

	doc := decodeOneJSONError(t, "ytr hel --json key", got.stderr)
	if doc.Suggestion != "Did you mean: ytr help" {
		t.Errorf("suggestion = %q, want %q", doc.Suggestion, "Did you mean: ytr help")
	}
}

func TestHelpTopicCompletionOffersSubcommands(t *testing.T) {
	got := runProbe(t, []string{"__complete", "help", ""})

	for _, want := range []string{"issue\t", "queue\t", "help\t"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout = %q, want it to offer %q", got.stdout, want)
		}
	}
}

func TestHelpTopicCompletionOffersNothingForAnUnknownTopic(t *testing.T) {
	got := runProbe(t, []string{"__complete", "help", "nosuch", ""})

	// Only cobra's trailing directive line is expected.
	if offered := strings.TrimSpace(strings.TrimSuffix(got.stdout, ":4\n")); offered != "" {
		t.Errorf("stdout = %q, want no completions for an unresolvable topic", got.stdout)
	}
}

func TestJSONCompletionOffersTheFieldsSetOnACommandOutsideTheRunner(t *testing.T) {
	got := runProbe(t, []string{"__complete", "issue", "changelog", "--json", ""})

	offered, _, _ := strings.Cut(got.stdout, "\n:")
	if fields := strings.Split(offered, "\n"); !slices.Equal(fields, issue.IssueChangelogFields) {
		t.Errorf("completion offers %q, want issue changelog's fields %q", fields, issue.IssueChangelogFields)
	}
}

func TestDispatchOnlyCommandsDoNotAdvertiseTheirBareForm(t *testing.T) {
	// The contract gives every group a RunE so cobra stops answering a bare
	// group with help and exit 0. Cobra prints a `ytr issue [flags]` usage line
	// for anything it considers runnable, and running that line exits 1.
	for _, argv := range [][]string{{"--help"}, {"issue", "--help"}, {"completion", "--help"}} {
		label := "ytr " + strings.Join(argv, " ")
		got := runProbe(t, argv)

		path := strings.Join(argv[:len(argv)-1], " ")
		bare := "\n  ytr " + strings.TrimSuffix(path+" ", " ") + " [flags]\n"
		if strings.Contains(got.stdout, bare) {
			t.Errorf("%s: help advertises %q, which exits 1", label, strings.TrimSpace(bare))
		}
		if !strings.Contains(got.stdout, "[command]") {
			t.Errorf("%s: help no longer shows the [command] usage line", label)
		}
	}
}

func TestLeafCommandsStillAdvertiseTheirFlags(t *testing.T) {
	got := runProbe(t, []string{"issue", "list", "--help"})

	if !strings.Contains(got.stdout, "\n  ytr issue list [flags]\n") {
		t.Errorf("stdout = %q, want the usage line for a command that really runs", got.stdout)
	}
}
