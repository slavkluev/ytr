package issue

import (
	"context"
	"fmt"
	"strings"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/errors"
)

// transitionRequest holds --to, which names the transition to execute rather
// than going to Tracker as a body.
type transitionRequest struct {
	To *string `json:"to,omitempty"`
}

type transitionResult struct {
	Key        string `json:"key"`
	Transition string `json:"transition"`
}

func newTransitionCmd() *cobra.Command {
	return runner.Write[transitionRequest, transitionResult, transitionResult]{
		Use:   "transition ISSUE-KEY",
		Short: "Transition issue status",
		Long: `Transition a Yandex Tracker issue to a new status. Uses a two-step flow:
fetches available transitions, matches the target by key or display name, then executes.`,
		Example: `  # Transition by display name
  ytr issue transition PROJ-123 --to "In Progress"

  # Transition by status key
  ytr issue transition PROJ-123 --to inProgress

  # Only the issue key and transition
  ytr issue transition PROJ-123 --to "Done" --json key,transition`,
		Args:     []runner.Arg{runner.IssueKey},
		Flags:    []runner.Flag{runner.Text("to", "Target status key or display name (required)")},
		Required: []string{"to"},
		Call:     executeTransition,
		Item:     func(result transitionResult) transitionResult { return result },
	}.Command()
}

func executeTransition(
	ctx context.Context, c *tracker.Client, args []string, req *transitionRequest,
) (transitionResult, error) {
	issueKey, to := args[0], api.DerefString(req.To, "")

	transitions, _, err := c.Issues.GetTransitions(ctx, issueKey)
	if err != nil {
		return transitionResult{}, err
	}

	matched := matchTransition(transitions, to)
	if matched == nil {
		return transitionResult{}, buildTransitionError(issueKey, to, transitions)
	}

	_, _, err = c.Issues.ExecuteTransition(ctx, issueKey, api.DerefFlexString(matched.ID, ""), nil)
	if err != nil {
		return transitionResult{}, err
	}

	return transitionResult{
		Key:        issueKey,
		Transition: api.DerefString(matched.To.Display, api.DerefString(matched.To.Key, to)),
	}, nil
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
