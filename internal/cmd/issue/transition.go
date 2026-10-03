package issue

import (
	"fmt"
	"io"
	"strings"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

// IssueTransitionFields lists the available JSON field names for issue transition output.
var IssueTransitionFields = []string{"key", "transition"}

type transitionResult struct {
	Key        string `json:"key"`
	Transition string `json:"transition"`
}

var transitionBody = validate.Body{Flags: []validate.BodyFlag{{Name: "to", Key: "to"}}, Required: []string{"to"}}

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
			if err := validate.ValidateIssueKey(args[0]); err != nil {
				return err
			}
			return transitionBody.CheckFlags(cmd.Flags().Changed)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTransition(cmd, args[0], toFlag)
		},
	}

	cmd.Flags().StringVar(&toFlag, "to", "", "Target status key or display name (required)")

	runner.SetFields(cmd, IssueTransitionFields)

	return cmd
}

func runTransition(cmd *cobra.Command, issueKey, toFlag string) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "issue transition", IssueTransitionFields)
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = IssueTransitionFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, IssueTransitionFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, IssueTransitionFields)
	}

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	transitioner := newTransitioner(auth)

	transitions, _, err := transitioner.GetTransitions(cmd.Context(), issueKey)
	if err != nil {
		return api.MapAPIError(err)
	}

	matched := matchTransition(transitions, toFlag)

	if matched == nil {
		return buildTransitionError(issueKey, toFlag, transitions)
	}

	_, _, err = transitioner.ExecuteTransition(cmd.Context(), issueKey, api.DerefFlexString(matched.ID, ""), nil)
	if err != nil {
		return api.MapAPIError(err)
	}

	targetDisplay := api.DerefString(matched.To.Display, api.DerefString(matched.To.Key, toFlag))
	return renderTransitionOutput(cmd.OutOrStdout(), opts, issueKey, targetDisplay)
}

func matchTransition(transitions []*tracker.Transition, toFlag string) *tracker.Transition {
	for _, t := range transitions {
		if t.To != nil && t.To.Key != nil && *t.To.Key == toFlag {
			return t
		}
	}

	for _, t := range transitions {
		if t.To != nil && t.To.Display != nil && strings.EqualFold(*t.To.Display, toFlag) {
			return t
		}
	}

	return nil
}

func renderTransitionOutput(w io.Writer, opts *output.Options, issueKey, targetDisplay string) error {
	if opts.IsJSON() {
		result := transitionResult{
			Key:        issueKey,
			Transition: targetDisplay,
		}
		if opts.HasFieldSelection() {
			filtered := output.FilterFields(result, opts.JSONFields)
			if opts.JQFilter != "" {
				return output.ApplyJQ(w, filtered, opts.JQFilter)
			}
			return opts.PrintJSON(w, filtered)
		}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, result, opts.JQFilter)
		}
		return opts.PrintJSON(w, result)
	}

	if opts.Quiet {
		output.PrintQuiet(w, issueKey)
		return nil
	}

	_, err := fmt.Fprintf(w, "%s transitioned to %s\n", issueKey, targetDisplay)
	return err
}

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
