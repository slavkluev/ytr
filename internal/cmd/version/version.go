// Package version provides the ytr version command.
package version

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
	ver "github.com/slavkluev/ytr/internal/version"
)

// VersionFields lists the available JSON field names for version output.
var VersionFields = []string{"version", "commit", "date", "goVersion", "os", "arch"}

// NewCmd creates the version command that displays build information.
// In human mode it prints a multi-line summary; with --json it outputs
// structured JSON with version, commit, date, goVersion, os, and arch fields.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show ytr version information",
		Long: `Display the version, commit hash, build date, Go version, and platform of the ytr binary.

JSON FIELDS
  version, commit, date, goVersion, os, arch`,
		Example: `  # Show version
  ytr version

  # Get version as JSON
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

	info := ver.Get()

	if opts.IsJSON() {
		return runner.PrintJSON(cmd, opts, output.FilterFields(info, opts.JSONFields))
	}

	return runner.PrintText(cmd, func(w io.Writer) error {
		if opts.Quiet {
			output.PrintQuiet(w, info.Version)
			return nil
		}

		_, _ = fmt.Fprintf(w, "ytr version %s\n", info.Version)
		_, _ = fmt.Fprintf(w, "commit: %s\n", info.Commit)
		_, _ = fmt.Fprintf(w, "date: %s\n", info.Date)
		_, _ = fmt.Fprintf(w, "go: %s\n", info.GoVersion)
		_, _ = fmt.Fprintf(w, "os/arch: %s/%s\n", info.OS, info.Arch)
		return nil
	})
}
