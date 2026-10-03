package cmd

import (
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

func TestIssueChangelog(t *testing.T) {
	runLeafRows(t, []leafRow{
		{
			name: "Not an issue key", args: []string{"issue", "changelog", "123"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "123"`},
		},
		{
			name: "Bad arg before the hint", args: []string{"issue", "changelog", "123", "--json="},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key "123"`},
		},
	})
}
