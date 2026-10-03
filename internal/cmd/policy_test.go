package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

// The names no command, flag or JSON field answers to.
const (
	unknownCommandProbe = "ytr-no-such-command"
	unknownFlagProbe    = "--ytr-no-such-flag"
	unknownFieldProbe   = "ytr-no-such-field"
)

// maxProbeArgCount is how far the suite looks for a positional argument count a
// command rejects. The widest validator in the tree takes two arguments, so
// three also covers "one too many".
const maxProbeArgCount = 3

const trackerFailureText = "faketracker answers every request with this error"

func failingTracker(t *testing.T) *faketracker.Fake {
	t.Helper()

	return faketracker.Failing(t, http.StatusInternalServerError, trackerFailureText)
}

// policyTarget is one command of the tree and what the suite derives from it.
type policyTarget struct {
	cmd    *cobra.Command
	path   []string
	fields []string

	// inv is the invocation of a leaf's Example, when one invokes the leaf, and
	// example what it did signed in with --jq . against failingTracker.
	inv        *invocation
	example    cliResult
	runExample []string
}

func (p *policyTarget) label() string { return p.cmd.CommandPath() }

func (p *policyTarget) isLeaf() bool { return !p.cmd.HasSubCommands() }

func (p *policyTarget) reachesTracker() bool { return p.inv != nil && len(p.example.Requests) > 0 }

// policy is a property every command it applies to must have.
type policy struct {
	name    string
	applies func(*policyTarget) bool
	check   func(*testing.T, *policyTarget)
}

var policies = []policy{
	{"Missing or unknown subcommand", func(p *policyTarget) bool { return !p.isLeaf() }, checkSubcommandProbes},
	{"Unknown flag", func(*policyTarget) bool { return true }, checkUnknownFlag},
	{"Argument count", func(p *policyTarget) bool { return len(rejectedArgCounts(p.cmd)) > 0 }, checkArgCounts},
	{"Debug", func(*policyTarget) bool { return true }, checkDebugForms},
	{"Args", (*policyTarget).isLeaf, checkArgsDeclared},
	{"Example", (*policyTarget).isLeaf, checkExample},
	{"Field hint", func(p *policyTarget) bool { return p.fields != nil && p.inv != nil }, checkFieldHint},
	{"Unknown field", func(p *policyTarget) bool { return p.fields != nil && p.inv != nil }, checkUnknownField},
	{"JSON FIELDS", (*policyTarget).isLeaf, checkJSONFieldsHelp},
	{"All with cursor", appliesAllWithCursor, checkAllWithCursor},
	{"Tracker 500", (*policyTarget).reachesTracker, checkTrackerFailure},
	{
		"Bad argument",
		func(p *policyTarget) bool { return p.reachesTracker() && len(p.inv.positional) > 0 },
		checkBadArgument,
	},
}

// TestEveryCommand derives every probe from the command itself, so a command
// added later is covered without this file being edited. Which properties
// apply is settled outside the subtests, so a -run pattern that picks one
// command does not leave the others' properties uncounted.
func TestEveryCommand(t *testing.T) {
	applied := make(map[string]int, len(policies))

	walkCommands(newRootCmd(&output.Options{}), func(cmd *cobra.Command) {
		target, ok := newPolicyTarget(t, cmd)
		if !ok {
			return
		}

		var checks []policy
		for _, p := range policies {
			if p.applies(target) {
				applied[p.name]++
				checks = append(checks, p)
			}
		}

		t.Run(target.label(), func(t *testing.T) {
			for _, p := range checks {
				t.Run(p.name, func(t *testing.T) { p.check(t, target) })
			}
		})
	})

	for _, p := range policies {
		if applied[p.name] == 0 {
			t.Errorf("%q applies to no command, so the suite no longer checks it", p.name)
		}
	}
}

func newPolicyTarget(t *testing.T, cmd *cobra.Command) (*policyTarget, bool) {
	t.Helper()

	target := &policyTarget{cmd: cmd, path: argPath(cmd)}

	// Every probe selects the command by this path, so a path that selects
	// another command would probe that one instead.
	if found, rest, err := cmd.Root().Find(target.path); err != nil || found != cmd || len(rest) > 0 {
		t.Errorf("%s: its path %q selects %v (rest %q, error %v)", target.label(), target.path, found, rest, err)
		return nil, false
	}

	if fields, ok := runner.Fields(cmd); ok {
		target.fields = fields
	}

	if inv, ok := exampleInvocation(cmd); ok {
		target.inv = &inv
		target.runExample = slices.Concat(inv.args, []string{"--jq", "."})
		target.example = runSignedIn(t, failingTracker(t), target.runExample...)
	}

	return target, true
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

// commandLine renders argv as a shell line that would run it, so an empty or
// spaced argument shows in a failure.
func commandLine(argv []string) string {
	words := []string{"ytr"}
	for _, arg := range argv {
		if arg == "" || strings.ContainsFunc(arg, needsShellQuote) {
			arg = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		}
		words = append(words, arg)
	}

	return strings.Join(words, " ")
}

func needsShellQuote(r rune) bool {
	safe := 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' ||
		strings.ContainsRune("_@%+=:,./-", r)
	return !safe
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
// both ends of its accepted range: a validator that wants one rejects zero
// before anything else.
//
// The result is empty for a group, whose arguments the subcommand probes cover,
// and for a validator that rejects nothing, cobra.ArbitraryArgs, which is a
// deliberate declaration rather than an omission.
func rejectedArgCounts(cmd *cobra.Command) []int {
	if cmd.HasSubCommands() || cmd.Args == nil {
		return nil
	}

	var counts []int
	for count := range maxProbeArgCount + 1 {
		if cmd.Args(cmd, placeholderArgs(count)) != nil {
			counts = append(counts, count)
		}
	}

	return counts
}

// badInvocations returns every mistake the suite appends to cmd's path: no
// subcommand and one that does not exist for a group, an unknown flag for any
// command, and each argument count a leaf's validator rejects.
func badInvocations(cmd *cobra.Command) [][]string {
	var mistakes [][]string
	if cmd.HasSubCommands() {
		mistakes = append(mistakes, nil, []string{unknownCommandProbe})
	}
	mistakes = append(mistakes, []string{unknownFlagProbe})
	for _, count := range rejectedArgCounts(cmd) {
		mistakes = append(mistakes, placeholderArgs(count))
	}

	return mistakes
}

func checkSubcommandProbes(t *testing.T, p *policyTarget) {
	doc := assertRejected(t, p.path)
	if !strings.HasPrefix(doc.Message, strconv.Quote(p.label())+" needs a subcommand: ") {
		t.Errorf("%s: message %q does not say a subcommand is missing", p.label(), doc.Message)
	}
	if want := fmt.Sprintf("Run %q for details.", p.label()+" --help"); doc.Suggestion != want {
		t.Errorf("%s: suggestion %q, want %q", p.label(), doc.Suggestion, want)
	}

	doc = assertRejected(t, slices.Concat(p.path, []string{unknownCommandProbe}))
	if want := fmt.Sprintf("unknown command %q for %q", unknownCommandProbe, p.label()); doc.Message != want {
		t.Errorf("%s: message %q, want %q", p.label(), doc.Message, want)
	}
	if doc.Suggestion == "" {
		t.Errorf("%s %s: suggestion is empty", p.label(), unknownCommandProbe)
	}
}

func checkUnknownFlag(t *testing.T, p *policyTarget) {
	doc := assertRejected(t, slices.Concat(p.path, []string{unknownFlagProbe}))
	if !strings.Contains(doc.Message, unknownFlagProbe) {
		t.Errorf("%s: message %q does not name the rejected flag", p.label(), doc.Message)
	}
	if doc.Suggestion == "" {
		t.Errorf("%s %s: suggestion is empty", p.label(), unknownFlagProbe)
	}
}

func checkArgCounts(t *testing.T, p *policyTarget) {
	// A leaf that takes no argument at all has to say which one it did not
	// expect, or the caller cannot tell a stray word from a misplaced value.
	takesNone := p.cmd.Args(p.cmd, nil) == nil && p.cmd.Args(p.cmd, placeholderArgs(1)) != nil

	for _, count := range rejectedArgCounts(p.cmd) {
		args := placeholderArgs(count)
		doc := assertRejected(t, slices.Concat(p.path, args))

		if doc.Suggestion == "" {
			t.Errorf("%s with %d argument(s): suggestion is empty", p.label(), count)
		}
		if takesNone && !strings.Contains(doc.Message, args[0]) {
			t.Errorf("%s with %d argument(s): message %q does not name the stray %q",
				p.label(), count, doc.Message, args[0])
		}
	}
}

// checkDebugForms runs every bad invocation of the command under --debug, with
// --json read before the mistake and with --json reachable only by re-reading
// the raw arguments because the mistake stopped flag parsing.
func checkDebugForms(t *testing.T, p *policyTarget) {
	for _, mistake := range badInvocations(p.cmd) {
		for _, argv := range [][]string{
			slices.Concat(p.path, []string{"--debug", "--json", "key"}, mistake),
			slices.Concat(p.path, []string{"--debug"}, mistake, []string{"--json", "key"}),
		} {
			label := commandLine(argv)
			got := runProbe(t, argv)

			if got.Code != ytrerrors.ExitUserError {
				t.Errorf("%s: exit = %d, want %d (stderr: %s)", label, got.Code, ytrerrors.ExitUserError, got.Stderr)
			}
			assertEmpty(t, label+": stdout", got.Stdout)
			decodeOneJSONError(t, label, withoutDebugLines(got.Stderr))
		}
	}
}

func checkArgsDeclared(t *testing.T, p *policyTarget) {
	if p.cmd.Args == nil {
		t.Errorf("%s declares no Args, so cobra accepts and silently ignores any positional argument; "+
			"declare cobra.NoArgs, cobra.ExactArgs(n) or cobra.ArbitraryArgs", p.label())
	}
}

// checkExample wants a line of the Example to invoke the leaf, and that line,
// signed in, to either reach Tracker or succeed without it.
func checkExample(t *testing.T, p *policyTarget) {
	if p.inv == nil {
		t.Errorf("%s: no Example line starts with `ytr` and invokes it", p.label())
		return
	}

	if len(p.example.Requests) == 0 && p.example.Code != ytrerrors.ExitSuccess {
		t.Errorf("%s: its Example, run as %s, fails before reaching Tracker: exit %d, %s",
			p.label(), commandLine(p.runExample), p.example.Code, p.example.Stderr)
	}
}

func checkFieldHint(t *testing.T, p *policyTarget) {
	argv := slices.Concat(p.inv.args, []string{"--json="})
	label := commandLine(argv)
	got := runProbe(t, argv)

	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("%s: exit = %d, want %d", label, got.Code, ytrerrors.ExitUserError)
	}
	assertEmpty(t, label+": stdout", got.Stdout)
	if want := fieldHint(strings.Join(p.path, " "), p.fields); got.Stderr != want {
		t.Errorf("%s: stderr = %q,\nwant %q", label, got.Stderr, want)
	}
}

func checkUnknownField(t *testing.T, p *policyTarget) {
	argv := slices.Concat(p.inv.args, []string{"--json", unknownFieldProbe})
	label := commandLine(argv)
	got := runProbe(t, argv)

	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("%s: exit = %d, want %d", label, got.Code, ytrerrors.ExitUserError)
	}
	assertEmpty(t, label+": stdout", got.Stdout)

	doc := decodeOneJSONErrorCoded(t, label, got.Stderr, ytrerrors.CodeInvalidField)
	if doc.InvalidField != unknownFieldProbe {
		t.Errorf("%s: invalidField = %q, want %q", label, doc.InvalidField, unknownFieldProbe)
	}
}

// checkJSONFieldsHelp wants --help to list under JSON FIELDS exactly the
// fields --json accepts, in order, and a leaf without fields to have no such
// section.
func checkJSONFieldsHelp(t *testing.T, p *policyTarget) {
	got := runProbe(t, slices.Concat(p.path, []string{"--help"}))

	listed, found := jsonFieldsSection(got.Stdout)
	switch {
	case p.fields == nil && found:
		t.Errorf("%s --help lists JSON FIELDS %q, but --json accepts no field", p.label(), listed)
	case p.fields != nil && !slices.Equal(listed, p.fields):
		t.Errorf("%s --help lists JSON FIELDS %q, want %q", p.label(), listed, p.fields)
	}
}

// jsonFieldsSection returns the comma-separated names under a JSON FIELDS
// heading of help, which may wrap over several indented lines.
func jsonFieldsSection(help string) ([]string, bool) {
	_, section, found := strings.Cut(help, "\nJSON FIELDS\n")
	if !found {
		return nil, false
	}

	var joined []string
	for line := range strings.Lines(section) {
		if !strings.HasPrefix(line, "  ") {
			break
		}
		joined = append(joined, strings.TrimSpace(line))
	}

	var fields []string
	for field := range strings.SplitSeq(strings.Join(joined, " "), ",") {
		fields = append(fields, strings.TrimSpace(field))
	}

	return fields, true
}

func appliesAllWithCursor(p *policyTarget) bool {
	return p.inv != nil && p.cmd.Flags().Lookup("all") != nil && p.cmd.Flags().Lookup("cursor") != nil
}

// checkAllWithCursor wants the pair refused instead of --all winning silently.
// The run is signed out, so a leaf that checked auth first would exit 3.
func checkAllWithCursor(t *testing.T, p *policyTarget) {
	argv := slices.Concat(p.inv.args, []string{"--all", "--cursor", "2"})
	label := commandLine(argv)
	got := runProbe(t, argv)

	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("%s: exit = %d, want %d (stderr: %s)", label, got.Code, ytrerrors.ExitUserError, got.Stderr)
	}
	assertEmpty(t, label+": stdout", got.Stdout)
	if !strings.Contains(got.Stderr, "cannot combine --all with --cursor") {
		t.Errorf("%s: stderr = %q, want it to name the conflict", label, got.Stderr)
	}
}

// checkTrackerFailure wants a Tracker error to leave stdout empty and reach
// stderr as one JSON document carrying the server's text and nothing else,
// with or without --debug lines beside it.
func checkTrackerFailure(t *testing.T, p *policyTarget) {
	debugArgs := slices.Concat(p.runExample, []string{"--debug"})
	runs := []struct {
		args []string
		res  cliResult
	}{
		{p.runExample, p.example},
		{debugArgs, runSignedIn(t, failingTracker(t), debugArgs...)},
	}

	for _, run := range runs {
		label, got := commandLine(run.args), run.res

		if got.Code != ytrerrors.ExitUserError {
			t.Errorf("%s: exit = %d, want %d (stderr: %s)", label, got.Code, ytrerrors.ExitUserError, got.Stderr)
		}
		assertEmpty(t, label+": stdout", got.Stdout)

		doc := decodeOneJSONError(t, label, withoutDebugLines(got.Stderr))
		if doc.Message != trackerFailureText {
			t.Errorf("%s: message = %q, want the server's text %q", label, doc.Message, trackerFailureText)
		}
	}
}

// checkBadArgument empties each positional argument of the Example in turn.
// No Tracker key, ID or name is empty, so every leaf must refuse it before
// sending a request, whichever validator it declares.
func checkBadArgument(t *testing.T, p *policyTarget) {
	for _, i := range p.inv.positional {
		argv := slices.Clone(p.runExample)
		argv[i] = ""
		label := commandLine(argv)

		got := runSignedIn(t, failingTracker(t), argv...)
		if len(got.Requests) > 0 {
			t.Errorf("%s: sent %+v, want no request", label, got.Requests)
		}
		if got.Code != ytrerrors.ExitUserError {
			t.Errorf("%s: exit = %d, want %d (stderr: %s)", label, got.Code, ytrerrors.ExitUserError, got.Stderr)
		}
		assertEmpty(t, label+": stdout", got.Stdout)
		decodeOneJSONError(t, label, got.Stderr)
	}
}

// invocation is how a leaf's Example runs it, less the output flags.
type invocation struct {
	// args select the leaf and carry the Example's flags and arguments.
	args []string

	// positional indexes the words of args that are neither a command name, a
	// flag nor a flag's value.
	positional []int
}

// exampleInvocation returns the first line of leaf's Example that starts with
// ytr and selects leaf, read as a shell would up to the first pipe,
// redirection or command separator.
func exampleInvocation(leaf *cobra.Command) (invocation, bool) {
	for line := range strings.Lines(leaf.Example) {
		words := shellWords(line)
		if len(words) < 2 || words[0] != "ytr" {
			continue
		}

		found, _, err := leaf.Root().Find(words[1:])
		if err != nil || found != leaf {
			continue
		}

		args := withoutOutputFlags(words[1:])

		return invocation{args: args, positional: positionals(leaf, args)}, true
	}

	return invocation{}, false
}

// shellWords splits line into words as a POSIX shell would, honouring single
// and double quotes and backslashes, and stops at the first unquoted |, <, >,
// ;, & or comment, which ends the command the line starts with.
func shellWords(line string) []string {
	var (
		words  []string
		word   strings.Builder
		inWord bool
		quote  rune
	)
	endWord := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case quote == '"':
			switch {
			case r == '"':
				quote = 0
			case r == '\\' && i+1 < len(runes) && strings.ContainsRune("\"\\$`", runes[i+1]):
				i++
				word.WriteRune(runes[i])
			default:
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == '\\' && i+1 < len(runes):
			i++
			word.WriteRune(runes[i])
			inWord = true
		case strings.ContainsRune("|<>;&", r), r == '#' && !inWord:
			endWord()
			return words
		case unicode.IsSpace(r):
			endWord()
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	endWord()

	return words
}

// withoutOutputFlags drops --json, --jq and --quiet with their values, which
// each property sets for itself.
func withoutOutputFlags(args []string) []string {
	var kept []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return append(kept, args[i:]...)
		case arg == "--json" || arg == "--jq":
			i++
		case arg == "--quiet",
			strings.HasPrefix(arg, "--json="), strings.HasPrefix(arg, "--jq="), strings.HasPrefix(arg, "--quiet="):
		default:
			kept = append(kept, arg)
		}
	}

	return kept
}

// positionals returns the indexes of the words of args, which select leaf,
// that are neither a command name, a flag nor a flag's value by leaf's own
// flag set.
func positionals(leaf *cobra.Command, args []string) []int {
	names := len(argPath(leaf))

	var found []int
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			for j := i + 1; j < len(args); j++ {
				found = append(found, j)
			}
			return found
		case strings.HasPrefix(arg, "--"):
			name, _, inline := strings.Cut(arg[2:], "=")
			if !inline && takesValue(lookupFlag(leaf, name)) {
				i++
			}
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			if shorthandTakesNextWord(leaf, arg[1:]) {
				i++
			}
		case names > 0:
			names--
		default:
			found = append(found, i)
		}
	}

	return found
}

// shorthandTakesNextWord reports whether the shorthand flags of cluster leave
// their value to the next word: the first one that takes a value takes the
// rest of the cluster, or the next word when it ends the cluster.
func shorthandTakesNextWord(leaf *cobra.Command, cluster string) bool {
	for i, c := range cluster {
		if takesValue(lookupShorthand(leaf, string(c))) {
			return i == len(cluster)-1
		}
	}

	return false
}

func lookupFlag(leaf *cobra.Command, name string) *pflag.Flag {
	if flag := leaf.Flags().Lookup(name); flag != nil {
		return flag
	}

	return leaf.InheritedFlags().Lookup(name)
}

func lookupShorthand(leaf *cobra.Command, shorthand string) *pflag.Flag {
	if flag := leaf.Flags().ShorthandLookup(shorthand); flag != nil {
		return flag
	}

	return leaf.InheritedFlags().ShorthandLookup(shorthand)
}

func takesValue(flag *pflag.Flag) bool {
	return flag != nil && flag.NoOptDefVal == ""
}

// runProbe runs argv signed out against a Tracker that fails every request, and
// fails t on any request: nothing a probe checks may reach Tracker.
func runProbe(t *testing.T, argv []string) cliResult {
	t.Helper()

	got := runSignedOut(t, failingTracker(t), argv...)
	if len(got.Requests) > 0 {
		t.Errorf("%s: sent %+v, want no request", commandLine(argv), got.Requests)
	}

	return got
}

// errorDocument is the JSON error shape a failed invocation must produce.
type errorDocument struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion"`
	InvalidField string `json:"invalidField"`
}

// assertRejected runs argv with a --json field selection appended after whatever
// it gets wrong, and fails unless the contract held: exit 1, nothing on stdout,
// and exactly one JSON error document on stderr. Appending --json last is what
// proves it is honoured even when the mistake stopped flag parsing first.
func assertRejected(t *testing.T, argv []string) errorDocument {
	t.Helper()

	probe := slices.Concat(argv, []string{"--json", "key"})
	label := commandLine(probe)
	got := runProbe(t, probe)

	if got.Code != ytrerrors.ExitUserError {
		t.Errorf("%s: exit = %d, want %d (stderr: %s)", label, got.Code, ytrerrors.ExitUserError, got.Stderr)
	}
	if got.Stdout != "" {
		t.Errorf("%s: stdout = %q, want empty", label, got.Stdout)
	}

	return decodeOneJSONError(t, label, got.Stderr)
}

// decodeOneJSONError fails unless stderr holds exactly one JSON error document
// carrying the user_error code and a message.
func decodeOneJSONError(t *testing.T, label, stderr string) errorDocument {
	t.Helper()

	return decodeOneJSONErrorCoded(t, label, stderr, ytrerrors.CodeUserError)
}

func decodeOneJSONErrorCoded(t *testing.T, label, stderr, code string) errorDocument {
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

	if doc.Code != code {
		t.Errorf("%s: code = %q, want %q", label, doc.Code, code)
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
	for line := range strings.SplitSeq(stderr, "\n") {
		if !strings.HasPrefix(line, "[debug] ") {
			kept = append(kept, line)
		}
	}

	return strings.Join(kept, "\n")
}
