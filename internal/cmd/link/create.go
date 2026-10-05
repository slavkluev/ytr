package link

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/validate"
)

func newCreateCmd() *cobra.Command {
	return runner.Write[tracker.LinkRequest, *tracker.IssueLink, linkItem]{
		Use:   "create ISSUE-KEY",
		Short: "Create a link to another issue",
		Long: `Create a typed link between two Yandex Tracker issues.

Provide --type and --issue for individual flags, or --from-json for full JSON input.`,
		Example: `  # Create a dependency link
  ytr link create PROJ-123 --type "depends on" --issue PROJ-456

  # Create link via JSON
  ytr link create PROJ-123 --from-json '{"relationship":"relates","issue":"PROJ-456"}'

  # Create a link and keep only its ID and type
  ytr link create PROJ-123 --type "relates" --issue PROJ-456 --json id,type`,
		Args: []runner.Arg{runner.IssueKey},
		Flags: []runner.Flag{
			runner.Text("type", `Link type (e.g., "depends on", "relates")`).Key("relationship"),
			runner.Text("issue", "Target issue key or ID (e.g., PROJ-456)").Check(validate.ValidateIssueKeyOrID),
		},
		FromJSON: `JSON input: inline '{"relationship":"...","issue":"..."}', @file, or - for stdin`,
		Required: []string{"relationship", "issue"},
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.LinkRequest,
		) (*tracker.IssueLink, error) {
			link, _, err := c.Issues.CreateLink(ctx, args[0], req)
			return link, err
		},
		Item: toLinkItem,
	}.Command()
}
