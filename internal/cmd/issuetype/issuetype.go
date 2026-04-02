// Package issuetype provides issue type commands for the ytr CLI.
package issuetype

import (
	"context"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
)

// issueTypeLister abstracts issue type list operations for testability.
type issueTypeLister interface {
	List(ctx context.Context) ([]*tracker.IssueType, *tracker.Response, error)
}

// newIssueTypeLister creates an issueTypeLister from resolved auth. Replaceable for testing.
var newIssueTypeLister = func(auth *config.ResolvedAuth) issueTypeLister {
	return api.NewClient(auth).IssueTypes
}

// NewCmd creates the parent "issuetype" command.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issuetype",
		Short: "Manage issue types",
		Long:  "View issue types in Yandex Tracker.",
	}
	cmd.AddCommand(newListCmd())
	return cmd
}
