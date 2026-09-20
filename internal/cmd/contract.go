package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

// maxSuggestionDistance is the edit distance within which a typed name is close
// enough to a real one to be named in a suggestion. It is cobra's default
// SuggestionsMinimumDistance, so a flag suggestion is as strict as the
// subcommand suggestions cobra produces.
const maxSuggestionDistance = 2

// helpCommandName is the name cobra gives the help command.
const helpCommandName = "help"

// dispatchOnlyAnnotation marks a command the contract made runnable: it has
// subcommands, nothing of its own to run, and a RunE that only reports the
// missing subcommand.
const dispatchOnlyAnnotation = "ytr:dispatch-only"

// installInvocationContract walks the tree rooted at cmd and makes every
// command that has subcommands reject an invocation it cannot serve.
//
// Two cobra defaults are the reason it exists. legacyArgs checks for an unknown
// subcommand on the root only, so `ytr issue lst` reaches the issue command with
// "lst" as a positional argument nobody reads. And a command with no Run is
// treated as a request for help: cobra writes the help text to stdout and
// returns nil, so `ytr issue` exits 0. Installing the contract by walking the
// tree keeps it in one place, and a group added later inherits it without
// anyone remembering to.
func installInvocationContract(cmd *cobra.Command) {
	// SuggestionsFor compares against this field, and cobra leaves it at 0 until
	// its own did-you-mean path runs, which the contract replaces. Without it
	// every suggestion would have to be an exact prefix.
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = maxSuggestionDistance
	}

	switch {
	case cmd.HasSubCommands():
		if cmd.Args == nil {
			cmd.Args = rejectUnknownSubcommand
		}

		if !cmd.Runnable() {
			cmd.RunE = reportMissingSubcommand
			markDispatchOnly(cmd)
		}
	case cmd.Args != nil:
		cmd.Args = explainArgsRejection(cmd.Args)
	}

	for _, sub := range cmd.Commands() {
		installInvocationContract(sub)
	}
}

// markDispatchOnly records that the contract, not the command itself, is what
// makes cmd runnable.
func markDispatchOnly(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string, 1)
	}

	cmd.Annotations[dispatchOnlyAnnotation] = "true"
}

// hideDispatchOnlyUsageLine keeps a command the contract made runnable from
// advertising its bare form. Cobra prints `ytr issue [flags]` for any command it
// considers runnable, and the contract makes every group runnable so cobra stops
// answering a bare group with help and exit 0 -- but running that line exits 1,
// so it is not an invocation to advertise. The `ytr issue [command]` line, the
// real one, stays.
//
// One installation on the root covers the tree: cobra looks the usage function
// up on the parent when a command has none of its own.
func hideDispatchOnlyUsageLine(rootCmd *cobra.Command) {
	renderUsage := rootCmd.UsageFunc()

	rootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		if cmd.Annotations[dispatchOnlyAnnotation] == "" {
			return renderUsage(cmd)
		}

		// Cobra asks Runnable() while it renders, and nothing else can answer it
		// differently for this one call.
		runE := cmd.RunE
		cmd.RunE = nil
		defer func() { cmd.RunE = runE }()

		return renderUsage(cmd)
	})
}

// explainArgsRejection wraps a leaf's own Args validator so a rejected argument
// carries a suggestion, as every other rejection the contract adds does.
// Cobra's validators return a bare error: `ytr issue list MTP` named the
// argument but left the caller nothing to run.
func explainArgsRejection(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		err := validate(cmd, args)
		if err == nil {
			return nil
		}

		// A validator that already reports an ExitError has said its piece.
		var exitErr *ytrerrors.ExitError
		if errors.As(err, &exitErr) {
			return err
		}

		return ytrerrors.NewUserError(err.Error(), helpSuggestion(cmd))
	}
}

// rejectUnknownSubcommand is the Args validator of every command that has
// subcommands: whatever positional argument reached it was meant to be a
// subcommand name. A command with no arguments is left to
// reportMissingSubcommand, which runs next and can list the alternatives.
func rejectUnknownSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}

	return unknownSubcommandError(cmd, args[0], args[1:])
}

// reportMissingSubcommand is the RunE of every command that has subcommands and
// nothing of its own to run. It exists so cobra sees the command as runnable:
// otherwise cobra answers with the help text and exit 0.
func reportMissingSubcommand(cmd *cobra.Command, _ []string) error {
	message := fmt.Sprintf(
		"%q needs a subcommand: %s",
		cmd.CommandPath(),
		strings.Join(availableSubcommands(cmd), ", "),
	)

	return ytrerrors.NewUserError(message, helpSuggestion(cmd))
}

// unknownSubcommandError reports name as not being a subcommand of cmd, naming
// the closest real subcommand when one is close enough.
//
// rest is whatever the caller typed after name. It is carried into the
// suggestion untouched, so the suggested command can be run as it stands:
// dropping it turned `ytr issue vew PROJ-1` into a suggestion that fails with
// "accepts 1 arg(s), received 0".
func unknownSubcommandError(cmd *cobra.Command, name string, rest []string) error {
	message := fmt.Sprintf("unknown command %q for %q", name, cmd.CommandPath())

	if closest := closestName(name, suggestionCandidates(cmd, name)); closest != "" {
		suggested := slices.Concat([]string{cmd.CommandPath(), closest}, rest)
		return ytrerrors.NewUserError(message, "Did you mean: "+strings.Join(suggested, " "))
	}

	return ytrerrors.NewUserError(message, helpSuggestion(cmd))
}

// suggestionCandidates returns the subcommand names close enough to typed to be
// suggested. cobra's SuggestionsFor leaves the help command out through
// IsAvailableCommand, so `ytr hel` came back with no did-you-mean at all.
func suggestionCandidates(cmd *cobra.Command, typed string) []string {
	candidates := cmd.SuggestionsFor(typed)

	if hasHelpCommand(cmd) && !slices.Contains(candidates, helpCommandName) &&
		isNear(typed, helpCommandName) {
		candidates = append(candidates, helpCommandName)
	}

	return candidates
}

// hasHelpCommand reports whether cmd dispatches to a help command.
func hasHelpCommand(cmd *cobra.Command) bool {
	for _, sub := range cmd.Commands() {
		if sub.Name() == helpCommandName {
			return true
		}
	}

	return false
}

// isNear applies cobra's suggestion rule: within maxSuggestionDistance, or a
// name that typed is a prefix of.
func isNear(typed, name string) bool {
	return editDistance(typed, name) <= maxSuggestionDistance ||
		strings.HasPrefix(strings.ToLower(name), strings.ToLower(typed))
}

// flagError turns a flag parse failure into an ExitError, so the failure is
// rendered by the one error renderer and reaches JSON mode like any other.
// Cobra looks the handler up on the root, so installing it there covers every
// command in the tree.
func flagError(cmd *cobra.Command, err error) error {
	suggestion := helpSuggestion(cmd)

	// Only an unknown flag leaves the user without a name to work from: a
	// missing value or a value of the wrong type already names its flag.
	var notExist *pflag.NotExistError
	if errors.As(err, &notExist) {
		if closest := closestFlag(cmd, notExist.GetSpecifiedName()); closest != "" {
			suggestion = "Did you mean: --" + closest
		}
	}

	return ytrerrors.NewUserError(err.Error(), suggestion)
}

// newHelpCmd builds ytr's own help command, replacing cobra's. Cobra's answers
// an unknown topic with a line on stdout and exit 0, and it cannot be fixed by
// reusing its resolution: once every command that has subcommands declares Args,
// Root().Find no longer fails on a name it cannot resolve. It returns the last
// command it did resolve plus the leftovers, so the leftovers are what say the
// topic was not found.
func newHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Long: `Help provides help for any command in the application.
Simply type ytr help [path to command] for full details.`,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeHelpTopics,
		RunE:              runHelp,
	}
}

// runHelp writes the help of the requested command, or fails when no command
// answers to the topic. An explicit help request is not a failure, so a
// resolved topic still exits 0.
func runHelp(cmd *cobra.Command, args []string) error {
	root := cmd.Root()
	target := root

	if len(args) > 0 {
		found, rest, err := root.Find(args)
		if err != nil || found == nil {
			return unknownSubcommandError(root, args[0], args[1:])
		}
		if len(rest) > 0 {
			return unknownSubcommandError(found, rest[0], rest[1:])
		}
		target = found
	}

	// Cobra adds these when it executes a command; help is shown without
	// executing the target, so they would be missing from its flag list.
	target.InitDefaultHelpFlag()
	target.InitDefaultVersionFlag()

	return target.Help()
}

// completeHelpTopics completes `ytr help <TAB>` with the subcommands of the
// command named so far.
func completeHelpTopics(
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]cobra.Completion, cobra.ShellCompDirective) {
	// Leftovers mean a name in args resolved to nothing, so there is no command
	// whose subcommands would be the topics to offer.
	target, rest, err := cmd.Root().Find(args)
	if err != nil || target == nil || len(rest) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []cobra.Completion
	for _, sub := range target.Commands() {
		if !isTypeable(sub) {
			continue
		}
		if strings.HasPrefix(sub.Name(), toComplete) {
			completions = append(completions, cobra.CompletionWithDesc(sub.Name(), sub.Short))
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// isTypeable reports whether sub is a name a caller may type after its parent.
// IsAvailableCommand leaves the help command out, and cobra's own help template
// puts it back with the same `or .IsAvailableCommand (eq .Name "help")` rule:
// `ytr --help` lists it under System, so the failure for a bare `ytr` has to
// list it too.
func isTypeable(sub *cobra.Command) bool {
	return sub.IsAvailableCommand() || sub.Name() == helpCommandName
}

// availableSubcommands returns the names a user may type after cmd, sorted.
func availableSubcommands(cmd *cobra.Command) []string {
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		if isTypeable(sub) {
			names = append(names, sub.Name())
		}
	}
	slices.Sort(names)

	return names
}

// helpSuggestion is the fallback suggestion: the one command that is always
// safe to run and always exits 0.
func helpSuggestion(cmd *cobra.Command) string {
	return fmt.Sprintf("Run %q for details.", cmd.CommandPath()+" --help")
}

// closestFlag returns the name of the flag of cmd closest to typed, or "" when
// none is close. Cobra exports SuggestionsFor for subcommands but keeps its edit
// distance private and has no flag equivalent, so this applies a comparable rule
// to flags: a name within maxSuggestionDistance that also kept most of what was
// typed, or a name that typed is a prefix of.
//
// Most of what was typed has to survive, or a mistyped one-letter shorthand
// would take the first short flag name it found: every one of them is within
// distance two of a single letter.
func closestFlag(cmd *cobra.Command, typed string) string {
	best := ""
	bestDistance := 0

	// Flag parsing has already merged the inherited persistent flags into this
	// set, which is why --json and --token are candidates too.
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		distance := editDistance(typed, flag.Name)
		near := distance <= maxSuggestionDistance && distance < utf8.RuneCountInString(typed)
		if !near && !strings.HasPrefix(flag.Name, typed) {
			return
		}
		if best == "" || distance < bestDistance || (distance == bestDistance && flag.Name < best) {
			best, bestDistance = flag.Name, distance
		}
	})

	return best
}

// closestName returns the candidate closest to typed, or "" when candidates is
// empty. Ties go to the lexicographically smaller name, so the suggestion is
// the same on every run.
func closestName(typed string, candidates []string) string {
	best := ""
	bestDistance := 0

	for _, candidate := range candidates {
		distance := editDistance(typed, candidate)
		if best == "" || distance < bestDistance || (distance == bestDistance && candidate < best) {
			best, bestDistance = candidate, distance
		}
	}

	return best
}

// editDistance returns the Levenshtein distance between a and b, ignoring case.
// Cobra computes the same distance for its subcommand suggestions but does not
// export it.
func editDistance(a, b string) int {
	from, to := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))

	previous := make([]int, len(to)+1)
	current := make([]int, len(to)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(from); i++ {
		current[0] = i
		for j := 1; j <= len(to); j++ {
			substitution := previous[j-1]
			if from[i-1] != to[j-1] {
				substitution++
			}
			current[j] = min(previous[j]+1, current[j-1]+1, substitution)
		}
		previous, current = current, previous
	}

	return previous[len(to)]
}
