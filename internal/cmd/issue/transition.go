package issue

import (
	"fmt"
	"io"
	"strings"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// TransitionFields lists the available JSON field names for issue transition output.
var TransitionFields = []string{"key", "transition"}

// transitionResult is a clean struct for JSON output of a successful transition.
type transitionResult struct {
	Key        string `json:"key"`
	Transition string `json:"transition"`
}

// newTransitionCmd creates the "issue transition" command for transitioning
// issue status via a two-step API flow: fetch available transitions, match
// the target, then execute.
func newTransitionCmd() *cobra.Command {
	var toFlag string

	cmd := &cobra.Command{
		Use:   "transition ISSUE-KEY",
		Short: "Transition issue status",
		Long: `Transition a Yandex Tracker issue to a new status. Uses a two-step flow:
fetches available transitions, matches the target by key or display name, then executes.

JSON FIELDS
  key, transition

SEE ALSO
  ytr issue view    - View issue details
  ytr issue update  - Update issue fields`,
		Example: `  # Transition by display name
  ytr issue transition PROJ-123 --to "In Progress"

  # Transition by status key
  ytr issue transition PROJ-123 --to inProgress

  # Get result as JSON
  ytr issue transition PROJ-123 --to "Done" --json key,transition`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validate.ValidateIssueKey(args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(cmd, args[0], toFlag)
		},
	}

	cmd.Flags().StringVar(&toFlag, "to", "", "Target status key or display name (required)")
	cmd.MarkFlagRequired("to") //nolint:errcheck // Cobra flag is known to exist

	jsonfields.Register("ytr issue transition", TransitionFields)

	return cmd
}

// runTransition executes the issue transition logic.
func runTransition(cmd *cobra.Command, issueKey, toFlag string) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue transition", TransitionFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = TransitionFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, TransitionFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, TransitionFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	transitioner := newTransitioner(auth)

	// Step 1: Fetch available transitions.
	transitions, _, err := transitioner.GetTransitions(cmd.Context(), issueKey)
	if err != nil {
		return api.MapAPIError(err)
	}

	// Step 2: Match --to value with two-pass strategy.
	matched := matchTransition(transitions, toFlag)

	// No match found -- return structured error with valid transitions.
	if matched == nil {
		return buildTransitionError(issueKey, toFlag, transitions)
	}

	// Step 3: Execute the transition.
	_, _, err = transitioner.ExecuteTransition(cmd.Context(), issueKey, api.DerefFlexString(matched.ID, ""), nil)
	if err != nil {
		return api.MapAPIError(err)
	}

	// Step 4: Output the result.
	targetDisplay := api.DerefString(matched.To.Display, api.DerefString(matched.To.Key, toFlag))
	return renderTransitionOutput(cmd.OutOrStdout(), issueKey, targetDisplay)
}

// matchTransition finds a transition matching toFlag by key (exact) or display name (case-insensitive).
func matchTransition(transitions []*tracker.Transition, toFlag string) *tracker.Transition {
	// Pass 1: Exact match on To.Key.
	for _, t := range transitions {
		if t.To != nil && t.To.Key != nil && *t.To.Key == toFlag {
			return t
		}
	}

	// Pass 2: Case-insensitive match on To.Display.
	for _, t := range transitions {
		if t.To != nil && t.To.Display != nil && strings.EqualFold(*t.To.Display, toFlag) {
			return t
		}
	}

	return nil
}

// renderTransitionOutput renders the transition result in JSON, quiet, or table mode.
func renderTransitionOutput(w io.Writer, issueKey, targetDisplay string) error {
	if output.IsJSON() {
		result := transitionResult{
			Key:        issueKey,
			Transition: targetDisplay,
		}
		if output.HasFieldSelection() {
			filtered := output.FilterFields(result, output.JSONFields)
			if output.JQFilter != "" {
				return output.ApplyJQ(w, filtered, output.JQFilter)
			}
			return output.PrintJSON(w, filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, result, output.JQFilter)
		}
		return output.PrintJSON(w, result)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, issueKey)
		return nil
	}

	// Table mode: human-readable message.
	_, err := fmt.Fprintf(w, "%s transitioned to %s\n", issueKey, targetDisplay)
	return err
}

// buildTransitionError creates a structured user error listing valid
// transitions and a copy-paste suggestion command.
func buildTransitionError(issueKey, toFlag string, transitions []*tracker.Transition) error {
	valid := make([]string, 0, len(transitions))
	for _, t := range transitions {
		name := api.DerefString(nil, "unknown")
		if t.To != nil {
			name = api.DerefString(t.To.Display, api.DerefString(t.To.Key, "unknown"))
		}
		valid = append(valid, name)
	}

	message := fmt.Sprintf("transition %q is not available for %s", toFlag, issueKey)

	suggestion := "Valid transitions: " + strings.Join(valid, ", ")
	if len(valid) > 0 {
		suggestion += fmt.Sprintf("\nTry: ytr issue transition %s --to %q", issueKey, valid[0])
	}

	return errors.NewUserError(message, suggestion)
}
