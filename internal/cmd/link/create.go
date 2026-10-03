package link

import (
	"context"
	"fmt"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/validate"
)

func newCreateCmd() *cobra.Command {
	return runner.Write[tracker.LinkRequest, *tracker.IssueLink, linkItem]{
		Use:   "create ISSUE-KEY",
		Short: "Create a link to another issue",
		Long: `Create a typed link between two Yandex Tracker issues.

Provide --type and --issue for individual flags, or --from-json for full JSON input.`,
		SeeAlso: `  ytr link list    - List links on issue
  ytr link delete  - Delete a link`,
		Example: `  # Create a dependency link
  ytr link create PROJ-123 --type "depends on" --issue PROJ-456

  # Create link via JSON
  ytr link create PROJ-123 --from-json '{"relationship":"relates","issue":"PROJ-456"}'

  # Create link and get result as JSON
  ytr link create PROJ-123 --type "relates" --issue PROJ-456 --json id,type`,
		Args: []runner.Arg{runner.IssueKey},
		Flags: []runner.Flag{
			runner.Text("type", `Link type (e.g., "depends on", "relates")`).Key("relationship"),
			runner.Text("issue", "Target issue key (e.g., PROJ-456)").Check(validate.ValidateIssueKey),
		},
		FromJSON: `JSON input: inline '{"relationship":"...","issue":"..."}', @file, or - for stdin`,
		Required: []string{"relationship", "issue"},
		Call: func(
			ctx context.Context, c *tracker.Client, args []string, req *tracker.LinkRequest,
		) (*tracker.IssueLink, error) {
			link, _, err := c.Issues.CreateLink(ctx, args[0], req)
			return link, err
		},
		Item:  toLinkItem,
		Quiet: linkID,
		Confirm: func(args []string, link *tracker.IssueLink) string {
			return fmt.Sprintf("Link %s created on %s", linkID(link), args[0])
		},
	}.Command()
}

func linkID(link *tracker.IssueLink) string {
	return api.DerefFlexString(link.ID, "")
}
