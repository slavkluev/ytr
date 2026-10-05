// Package version provides the ytr version command.
package version

import (
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
	ver "github.com/slavkluev/ytr/internal/version"
)

// VersionFields lists the available JSON field names for version output.
var VersionFields = []string{"version", "commit", "date", "goVersion", "os", "arch"}

// NewCmd returns the version command, which prints the build information as
// JSON: version, commit, date, goVersion, os, and arch.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show ytr version information",
		Long: `Display the version, commit hash, build date, Go version, and platform of the ytr binary.

JSON FIELDS
  version, commit, date, goVersion, os, arch`,
		Example: `  # Show version
  ytr version

  # Only the version and commit
  ytr version --json version,commit

  # Get just the version string
  ytr version --json version --jq '.version'`,
		Args: cobra.NoArgs,
		RunE: runVersion,
	}

	runner.SetFields(cmd, VersionFields)

	return cmd
}

func runVersion(cmd *cobra.Command, _ []string) error {
	opts, err := runner.SelectFields(cmd, VersionFields)
	if err != nil {
		return err
	}

	return runner.PrintJSON(cmd, opts, output.FilterFields(ver.Get(), opts.JSONFields))
}
