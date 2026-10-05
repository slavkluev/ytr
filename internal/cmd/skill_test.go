package cmd

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const skillPath = "../../skills/ytr/SKILL.md"

// badInvocationsHeading opens the SKILL.md section whose code blocks show
// invocations ytr rejects, so each of their invocations must be one it rejects.
const badInvocationsHeading = "### Bad invocations"

var (
	commandCell = regexp.MustCompile("^`(ytr(?: [^`]*)?)`$")
	flagSpan    = regexp.MustCompile("`(-[^`\\s=]+)[^`]*`")
)

// documentedLine is a line an agent may copy: from a SKILL.md code block or a
// command's Example.
type documentedLine struct {
	where string
	text  string

	// rejected marks a line documenting an invocation ytr refuses.
	rejected bool
}

func readSkill(t *testing.T) string {
	t.Helper()

	doc, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("reading SKILL.md: %v", err)
	}

	return string(doc)
}

// skillCodeLines returns the lines of SKILL.md's code blocks. Prose, inline code
// included, is not an invocation anyone runs as written.
func skillCodeLines(doc string) []documentedLine {
	var (
		lines             []documentedLine
		inCode, rejecting bool
	)

	for i, line := range strings.Split(doc, "\n") {
		switch {
		case strings.HasPrefix(strings.TrimSpace(line), "```"):
			inCode = !inCode
		case inCode:
			where := fmt.Sprintf("SKILL.md:%d", i+1)
			lines = append(lines, documentedLine{where: where, text: line, rejected: rejecting})
		case strings.HasPrefix(line, "#"):
			// Only a heading at the section's own level or above ends it.
			if level := len(line) - len(strings.TrimLeft(line, "#")); level <= 3 {
				rejecting = strings.TrimSpace(line) == badInvocationsHeading
			}
		}
	}

	return lines
}

// exampleLines returns the lines of every command's Example.
func exampleLines(root *cobra.Command) []documentedLine {
	var lines []documentedLine
	walkCommands(root, func(cmd *cobra.Command) {
		for line := range strings.Lines(cmd.Example) {
			lines = append(lines, documentedLine{where: cmd.CommandPath() + " Example", text: line})
		}
	})

	return lines
}

// invocationProblems checks every ytr command on lines without running it and
// returns one problem per command that does not do what its line documents,
// with the number of ytr commands it checked.
func invocationProblems(lines []documentedLine) ([]string, int) {
	var (
		problems []string
		checked  int
	)

	for _, line := range lines {
		// A line the shell cannot parse fails before ytr runs, which is what a
		// rejected line documents.
		commands, err := shellCommands(line.text)
		if err != nil {
			if !line.rejected {
				problems = append(problems, fmt.Sprintf("%s: %s: %v", line.where, strings.TrimSpace(line.text), err))
			}
			continue
		}

		for _, words := range commands {
			if words[0] != "ytr" {
				continue
			}
			checked++

			err := invocationError(words)
			switch {
			case line.rejected && err == nil:
				problems = append(problems, fmt.Sprintf("%s: %s is shown under %q, but ytr accepts it",
					line.where, commandLine(words[1:]), badInvocationsHeading))
			case !line.rejected && err != nil:
				problems = append(problems, fmt.Sprintf("%s: %s: %v", line.where, commandLine(words[1:]), err))
			}
		}
	}

	return problems, checked
}

// invocationError returns why words, a ytr command as a shell passes it, would
// fail before doing anything: no leaf selected, a flag its leaf does not parse,
// a positional argument count it refuses, or a --json field it does not have.
// Positional arguments go only to the leaf's Args validator, which counts them:
// a placeholder such as PROJ-123 is checked when the command runs, and this
// never runs it.
//
// Each call builds its own tree, because parsing writes the values into it.
func invocationError(words []string) error {
	cmd, rest, err := newRootCmd(&output.Options{}).Find(words[1:])
	if err != nil {
		return err
	}

	// Cobra adds --help when it executes a command, and Find does not execute.
	cmd.InitDefaultHelpFlag()
	if err = cmd.ParseFlags(rest); err != nil {
		return err
	}
	if err = cmd.ValidateFlagGroups(); err != nil {
		return err
	}
	positional := cmd.Flags().Args()

	if cmd.HasSubCommands() {
		if len(positional) > 0 {
			return unknownSubcommandError(cmd, positional[0], nil)
		}
		return reportMissingSubcommand(cmd, nil)
	}

	if err = cmd.ValidateArgs(positional); err != nil {
		return err
	}

	if !cmd.Flags().Changed("json") {
		return nil
	}

	selected, err := cmd.Flags().GetStringSlice("json")
	if err != nil {
		return err
	}
	fields, hasFields := runner.Fields(cmd)

	// A leaf with fields refuses a --json that names none, --jq or not, as
	// output.Options.HasEmptySelection has it.
	if hasFields && len(selected) == 0 {
		return fmt.Errorf("--json selects no field, which %s refuses", cmd.CommandPath())
	}

	if err = output.ValidateFields(selected, fields); err != nil {
		return fmt.Errorf("--json: %w; the fields of %s are %q", err, cmd.CommandPath(), fields)
	}

	return nil
}

// documentedWrite is a ytr command on a documented line whose leaf takes
// --from-json.
type documentedWrite struct {
	where string

	// args are the words after ytr.
	args []string

	// body is what the command gives --from-json, when passed says it gives it.
	body   string
	passed bool
}

// documentedWrites returns the ytr commands on lines that select a leaf taking
// --from-json. A line documented as rejected teaches no form, and one that
// does not parse is invocationProblems' to report.
func documentedWrites(lines []documentedLine) []documentedWrite {
	var writes []documentedWrite

	for _, line := range lines {
		commands, err := shellCommands(line.text)
		if err != nil || line.rejected {
			continue
		}

		for _, words := range commands {
			if words[0] != "ytr" {
				continue
			}

			cmd, rest, err := newRootCmd(&output.Options{}).Find(words[1:])
			if err != nil || cmd.Flags().Lookup(validate.FromJSONFlag) == nil || cmd.ParseFlags(rest) != nil {
				continue
			}

			body, _ := cmd.Flags().GetString(validate.FromJSONFlag)
			writes = append(writes, documentedWrite{
				where: line.where, args: words[1:], body: body, passed: cmd.Flags().Changed(validate.FromJSONFlag),
			})
		}
	}

	return writes
}

// flagFormWrites returns one problem per write on lines that does not pass
// --from-json, with the number of writes it checked.
func flagFormWrites(lines []documentedLine) ([]string, int) {
	writes := documentedWrites(lines)

	var problems []string
	for _, write := range writes {
		if !write.passed {
			problems = append(problems, fmt.Sprintf("%s: %s gives the body as flags; write it as --from-json",
				write.where, commandLine(write.args)))
		}
	}

	return problems, len(writes)
}

// unsentBodies runs every write on lines that gives --from-json its body
// inline, signed in against a Tracker that fails every request, and returns
// one problem per write that sends none: its body or the flags beside it were
// refused. It also returns the number of writes it ran. A body on stdin or in
// a file is not on the line to run.
func unsentBodies(t *testing.T, lines []documentedLine) ([]string, int) {
	t.Helper()

	var (
		problems []string
		ran      int
	)

	for _, write := range documentedWrites(lines) {
		if !write.passed || write.body == "-" || strings.HasPrefix(write.body, "@") {
			continue
		}
		ran++

		argv := slices.Concat(withoutOutputFlags(write.args), []string{"--jq", "."})
		if got := runSignedIn(t, failingTracker(t), argv...); len(got.Requests) == 0 {
			problems = append(problems, fmt.Sprintf("%s: %s sends no request: exit %d, %s",
				write.where, commandLine(write.args), got.Code, strings.TrimSpace(got.Stderr)))
		}
	}

	return problems, ran
}

// commandRow is a SKILL.md table line whose first cell is `ytr …`.
type commandRow struct {
	where string
	usage string

	// flags are the code spans of the Key Flags cell that name a flag, such as
	// --queue, without what follows the name.
	flags []string
}

func commandRows(doc string) []commandRow {
	var rows []commandRow
	for i, line := range strings.Split(doc, "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}

		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		match := commandCell.FindStringSubmatch(strings.TrimSpace(cells[0]))
		if match == nil {
			continue
		}

		row := commandRow{where: fmt.Sprintf("SKILL.md:%d", i+1), usage: match[1]}
		for _, span := range flagSpan.FindAllStringSubmatch(cells[len(cells)-1], -1) {
			row.flags = append(row.flags, span[1])
		}
		rows = append(rows, row)
	}

	return rows
}

// usageOf is what a command-table row of leaf reads: its path, then the
// arguments its Use names.
func usageOf(leaf *cobra.Command) string {
	if _, args, found := strings.Cut(leaf.Use, " "); found {
		return leaf.CommandPath() + " " + args
	}

	return leaf.CommandPath()
}

// commandTableProblems wants rows to hold exactly one row per non-hidden leaf
// of root, reading as the leaf's usage, whose Key Flags name only flags the
// leaf accepts and every flag the leaf defines itself.
func commandTableProblems(root *cobra.Command, rows []commandRow) []string {
	var (
		problems []string
		leaves   []*cobra.Command
		byUsage  = map[string]*cobra.Command{}
		rowsOf   = map[*cobra.Command][]string{}
	)

	walkCommands(root, func(cmd *cobra.Command) {
		if !cmd.HasSubCommands() && !cmd.Hidden {
			leaves = append(leaves, cmd)
			byUsage[usageOf(cmd)] = cmd
		}
	})

	for _, row := range rows {
		leaf := byUsage[row.usage]
		if leaf == nil {
			leaf = leafNamedBy(root, row.usage)
			if leaf == nil {
				problems = append(problems, fmt.Sprintf("%s: `%s` names no command", row.where, row.usage))
				continue
			}
			problems = append(problems, fmt.Sprintf("%s: `%s` should read `%s`, the usage of %s",
				row.where, row.usage, usageOf(leaf), leaf.CommandPath()))
		}

		rowsOf[leaf] = append(rowsOf[leaf], row.where)
		problems = append(problems, keyFlagProblems(leaf, row)...)
	}

	for _, leaf := range leaves {
		switch where := rowsOf[leaf]; len(where) {
		case 0:
			problems = append(problems, fmt.Sprintf("%s has no row in SKILL.md's command table; add `%s`",
				leaf.CommandPath(), usageOf(leaf)))
		case 1:
		default:
			problems = append(problems, fmt.Sprintf("%s has %d rows in SKILL.md's command table: %s",
				leaf.CommandPath(), len(where), strings.Join(where, ", ")))
		}
	}

	return problems
}

// leafNamedBy returns the leaf a row's usage selects when it does not read as
// that leaf's usage, so the row can be named as stale rather than unknown.
func leafNamedBy(root *cobra.Command, usage string) *cobra.Command {
	found, _, err := root.Find(strings.Fields(usage)[1:])
	if err != nil || found.HasSubCommands() || found.Hidden {
		return nil
	}

	return found
}

func keyFlagProblems(leaf *cobra.Command, row commandRow) []string {
	var problems []string

	named := map[string]bool{}
	for _, span := range row.flags {
		var flag *pflag.Flag
		if name, long := strings.CutPrefix(span, "--"); long {
			flag = lookupFlag(leaf, name)
		} else {
			flag = lookupShorthand(leaf, strings.TrimPrefix(span, "-"))
		}

		if flag == nil {
			problems = append(problems, fmt.Sprintf("%s: `%s` names %s, which %s does not accept",
				row.where, row.usage, span, leaf.CommandPath()))
			continue
		}
		named[flag.Name] = true
	}

	leaf.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		if !named[flag.Name] {
			problems = append(problems, fmt.Sprintf("%s: `%s` leaves out --%s, which %s defines",
				row.where, row.usage, flag.Name, leaf.CommandPath()))
		}
	})

	return problems
}

func TestSkillInvocationsRunAsWritten(t *testing.T) {
	t.Parallel()

	problems, checked := invocationProblems(skillCodeLines(readSkill(t)))
	if checked == 0 {
		t.Fatal("found no ytr invocation in SKILL.md's code blocks, so the check is not reaching them")
	}

	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestExampleInvocationsRunAsWritten(t *testing.T) {
	t.Parallel()

	problems, checked := invocationProblems(exampleLines(newRootCmd(&output.Options{})))
	if checked == 0 {
		t.Fatal("found no ytr invocation in any Example, so the check is not reaching them")
	}

	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestSkillWritesPassFromJSON(t *testing.T) {
	t.Parallel()

	problems, checked := flagFormWrites(skillCodeLines(readSkill(t)))
	if checked == 0 {
		t.Fatal("found no write in SKILL.md's code blocks, so the check is not reaching them")
	}

	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestExampleWritesPassFromJSON(t *testing.T) {
	t.Parallel()

	problems, checked := flagFormWrites(exampleLines(newRootCmd(&output.Options{})))
	if checked == 0 {
		t.Fatal("found no write in any Example, so the check is not reaching them")
	}

	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestDocumentedJSONBodiesReachTracker(t *testing.T) {
	t.Parallel()

	lines := slices.Concat(skillCodeLines(readSkill(t)), exampleLines(newRootCmd(&output.Options{})))

	problems, ran := unsentBodies(t, lines)
	if ran == 0 {
		t.Fatal("found no inline --from-json body in SKILL.md or any Example, so the check is not reaching them")
	}

	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestBodyCheckNamesTheBodyTrackerNeverGets(t *testing.T) {
	t.Parallel()

	rows := []struct {
		line string
		ran  int
		want string
	}{
		{line: `ytr comment create PROJ-1 --from-json '{"text":"x"}' --json id`, ran: 1},
		{line: "ytr comment create PROJ-1 --from-json @body.json"},
		{line: "ytr issue list --all --jq '{issues:[.items[].key]}' | ytr bulk update --from-json -"},
		{line: "ytr comment create PROJ-1 --body x"},
		{
			line: `ytr comment create PROJ-1 --body x --from-json '{}'`, ran: 1,
			want: "probe: ytr comment create PROJ-1 --body x --from-json '{}' sends no request: exit 1, " +
				`{"code":"user_error","message":"cannot combine --from-json with --body"`,
		},
		{
			line: `ytr comment create PROJ-1 --from-json '{"body":"x"}'`, ran: 1,
			want: `sends no request: exit 1, {"code":"invalid_field"`,
		},
		{
			line: `ytr issue create --from-json '{"queue":"PROJ"}'`, ran: 1,
			want: `sends no request: exit 1, {"code":"user_error","message":"missing --summary"`,
		},
		{
			line: `ytr issue update PROJ-1 --from-json '{"summary":'`, ran: 1,
			want: `sends no request: exit 1, {"code":"user_error","message":"invalid JSON input`,
		},
	}

	for _, row := range rows {
		t.Run(row.line, func(t *testing.T) {
			t.Parallel()

			problems, ran := unsentBodies(t, []documentedLine{{where: "probe", text: row.line}})

			if ran != row.ran {
				t.Errorf("ran = %d, want %d", ran, row.ran)
			}

			switch {
			case row.want == "" && len(problems) > 0:
				t.Errorf("problems = %q, want none", problems)
			case row.want != "" && (len(problems) != 1 || !strings.Contains(problems[0], row.want)):
				t.Errorf("problems = %q, want one containing %q", problems, row.want)
			}
		})
	}
}

func TestWriteFormCheckNamesTheFlagForm(t *testing.T) {
	t.Parallel()

	rows := []struct {
		line     string
		rejected bool
		checked  int
		want     string
	}{
		{line: `ytr comment create K-1 --from-json '{"text":"x"}'`, checked: 1},
		{line: "ytr comment create K-1 --from-json @body.json --json id", checked: 1},
		{
			line:    "ytr comment create K-1 --body x",
			checked: 1,
			want:    "probe: ytr comment create K-1 --body x gives the body as flags; write it as --from-json",
		},
		{
			line:    "ytr issue transition K-1 --to open",
			checked: 1,
			want:    "probe: ytr issue transition K-1 --to open gives the body as flags; write it as --from-json",
		},
		{
			line:    "ytr issue list --jq '.items[].key' | ytr bulk update --field a=b",
			checked: 1,
			want:    "probe: ytr bulk update --field a=b gives the body as flags; write it as --from-json",
		},
		{line: "ytr issue list --jq '{issues:[.items[].key]}' | ytr bulk update --from-json -", checked: 1},
		{line: "ytr comment create K-1 --body x", rejected: true},
		{line: "ytr comment delete K-1 5"},
		{line: "ytr issue view K-1"},
	}

	for _, row := range rows {
		t.Run(row.line, func(t *testing.T) {
			t.Parallel()

			problems, checked := flagFormWrites(
				[]documentedLine{{where: "probe", text: row.line, rejected: row.rejected}},
			)

			if checked != row.checked {
				t.Errorf("checked = %d, want %d", checked, row.checked)
			}

			var want []string
			if row.want != "" {
				want = []string{row.want}
			}
			if !slices.Equal(problems, want) {
				t.Errorf("problems = %q, want %q", problems, want)
			}
		})
	}
}

func TestSkillCommandTableCoversEveryLeaf(t *testing.T) {
	t.Parallel()

	rows := commandRows(readSkill(t))
	if len(rows) == 0 {
		t.Fatal("found no command row in SKILL.md, so the check is not reaching the table")
	}

	for _, problem := range commandTableProblems(newRootCmd(&output.Options{}), rows) {
		t.Error(problem)
	}
}

func TestSkillCheckNamesWhyAnInvocationFails(t *testing.T) {
	t.Parallel()

	rows := []struct {
		line     string
		rejected bool
		want     string
	}{
		{line: "echo X | ytr bulk update --field p=c"},
		{line: `ytr user get "$(ytr comment list K-1 --json authorId)"`},
		{line: "ytr help issue list"},
		{line: "ytr issue list --json KEY,Summary"},
		{line: "ytr issue lst", rejected: true},
		{line: "ytr issue list", rejected: true, want: `probe: ytr issue list is shown under "### Bad invocations"`},
		{
			line: "ytr issue list --filter assignee=me()",
			want: `probe: ytr issue list --filter assignee=me(): unquoted '(' at column 36`,
		},
		{line: "ytr issue lst", want: `probe: ytr issue lst: unknown command "lst" for "ytr issue"`},
		{line: "ytr issue --json key", want: `probe: ytr issue --json key: "ytr issue" needs a subcommand`},
		{line: "ytr", want: `probe: ytr: "ytr" needs a subcommand`},
		{line: "ytr issue list --limitt 5", want: "probe: ytr issue list --limitt 5: unknown flag: --limitt"},
		{line: "ytr auth status --json", want: "probe: ytr auth status --json: flag needs an argument: --json"},
		{
			line: "ytr issue list --json key,nosuch",
			want: `probe: ytr issue list --json key,nosuch: --json: unknown field: "nosuch"; the fields of ytr issue list are`,
		},
		{line: "ytr issue view K-1 K-2", want: "probe: ytr issue view K-1 K-2: accepts 1 arg(s), received 2"},
		{line: "ytr issue list --quiet", want: "probe: ytr issue list --quiet: unknown flag: --quiet"},
		{
			line: "ytr issue list --json ''",
			want: "probe: ytr issue list --json '': --json selects no field, which ytr issue list refuses",
		},
		{
			line: "ytr issue list --json=",
			want: "probe: ytr issue list --json=: --json selects no field, which ytr issue list refuses",
		},
		{
			line: "ytr issue list --json= --jq .items",
			want: "probe: ytr issue list --json= --jq .items: --json selects no field, which ytr issue list refuses",
		},
		{line: "ytr completion bash", want: `probe: ytr completion bash: unknown command "completion" for "ytr"`},
		{line: "ytr issue list K-1", want: `probe: ytr issue list K-1: unknown command "K-1" for "ytr issue list"`},
	}

	for _, row := range rows {
		t.Run(row.line, func(t *testing.T) {
			t.Parallel()

			problems, _ := invocationProblems(
				[]documentedLine{{where: "probe", text: row.line, rejected: row.rejected}},
			)

			if row.want == "" {
				if len(problems) > 0 {
					t.Errorf("problems = %q, want none", problems)
				}
				return
			}

			if len(problems) != 1 || !strings.Contains(problems[0], row.want) {
				t.Errorf("problems = %q, want one containing %q", problems, row.want)
			}
		})
	}
}

func TestSkillTableCheckNamesLeafAndFlag(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "leaf without a row",
			want: "ytr help has no row in SKILL.md's command table; add `ytr help [command]`",
		},
		{
			name: "stale usage",
			doc:  "| `ytr issue view KEY` | View | |",
			want: "SKILL.md:1: `ytr issue view KEY` should read `ytr issue view ISSUE-KEY`, the usage of ytr issue view",
		},
		{
			name: "no such command",
			doc:  "| `ytr issue lst` | List | |",
			want: "SKILL.md:1: `ytr issue lst` names no command",
		},
		{
			name: "missing local flag",
			doc:  "| `ytr issue transition ISSUE-KEY` | Transition | `--json` |",
			want: "SKILL.md:1: `ytr issue transition ISSUE-KEY` leaves out --to, which ytr issue transition defines",
		},
		{
			name: "flag the leaf does not accept",
			doc:  "| `ytr version` | Version | `--json`, `--limit N` |",
			want: "SKILL.md:1: `ytr version` names --limit, which ytr version does not accept",
		},
		{
			name: "two rows",
			doc:  "| `ytr version` | Version | |\n| `ytr version` | Version | |",
			want: "ytr version has 2 rows in SKILL.md's command table: SKILL.md:1, SKILL.md:2",
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			problems := commandTableProblems(newRootCmd(&output.Options{}), commandRows(row.doc))

			for _, problem := range problems {
				if problem == row.want {
					return
				}
			}
			t.Errorf("problems = %q, want one equal to %q", problems, row.want)
		})
	}
}

func TestSkillProblemsNameTheirLine(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name string
		doc  []string
		want []string
	}{
		{
			name: "code block line",
			doc:  []string{"# ytr", "", "```bash", "ytr version", "ytr issue lst", "```"},
			want: []string{`SKILL.md:5: ytr issue lst: unknown command "lst" for "ytr issue"`},
		},
		{
			name: "prose is not checked",
			doc:  []string{"Run `ytr issue lst`.", "ytr issue lst"},
		},
		{
			name: "accepted line under Bad invocations",
			doc:  []string{"### Bad invocations", "", "```bash", "ytr issue lst", "ytr issue list", "```"},
			want: []string{`SKILL.md:5: ytr issue list is shown under "### Bad invocations", but ytr accepts it`},
		},
		{
			name: "a deeper heading stays in the section",
			doc:  []string{"### Bad invocations", "#### Typos", "```bash", "ytr version", "```"},
			want: []string{`SKILL.md:4: ytr version is shown under "### Bad invocations", but ytr accepts it`},
		},
		{
			name: "a line the shell rejects under Bad invocations",
			doc:  []string{"### Bad invocations", "```bash", "ytr issue list --filter assignee=me()", "```"},
		},
		{
			name: "a line the shell rejects elsewhere",
			doc:  []string{"```bash", "ytr issue list --filter assignee=me()", "```"},
			want: []string{"SKILL.md:2: ytr issue list --filter assignee=me(): unquoted '(' at column 36"},
		},
		{
			name: "a comment in a code block is not a heading",
			doc:  []string{"### Bad invocations", "```bash", "# Error: unknown command", "ytr issue lst", "```"},
		},
		{
			name: "a ### heading ends the section",
			doc: []string{
				"### Bad invocations", "```bash", "ytr issue lst", "```",
				"### Next", "```bash", "ytr version", "ytr issue lst", "```",
			},
			want: []string{`SKILL.md:8: ytr issue lst: unknown command "lst" for "ytr issue"`},
		},
		{
			name: "a ## heading ends the section",
			doc: []string{
				"### Bad invocations", "```bash", "ytr issue lst", "```",
				"## Flags Reference", "```bash", "ytr issue lst", "ytr version", "```",
			},
			want: []string{`SKILL.md:7: ytr issue lst: unknown command "lst" for "ytr issue"`},
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()

			problems, _ := invocationProblems(skillCodeLines(strings.Join(row.doc, "\n")))

			if !slices.Equal(problems, row.want) {
				t.Errorf("problems = %q,\nwant %q", problems, row.want)
			}
		})
	}
}

func TestSkillExampleLinesNameTheirCommand(t *testing.T) {
	t.Parallel()

	root := newRootCmd(&output.Options{})
	lines := exampleLines(root)

	byWhere := map[string]string{}
	for _, line := range lines {
		byWhere[line.where] += line.text
	}

	examples := 0
	walkCommands(root, func(cmd *cobra.Command) {
		if cmd.Example == "" {
			return
		}
		examples++

		if got := byWhere[cmd.CommandPath()+" Example"]; got != cmd.Example {
			t.Errorf("lines named %q = %q, want its Example %q", cmd.CommandPath()+" Example", got, cmd.Example)
		}
	})
	if examples == 0 {
		t.Fatal("no command has an Example")
	}

	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line.text), "ytr ") {
			continue
		}

		line.text = strings.TrimSuffix(line.text, "\n") + " " + unknownFlagProbe
		problems, _ := invocationProblems([]documentedLine{line})
		if len(problems) != 1 || !strings.HasPrefix(problems[0], line.where+": ") {
			t.Errorf("problems = %q, want one starting with %q", problems, line.where+": ")
		}
		return
	}
	t.Error("found no Example line that starts with ytr")
}
