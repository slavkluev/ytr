// Package version provides the ytr version command.
package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/output"
	ver "github.com/slavkluev/ytr/internal/version"
)

// VersionFields lists the available JSON field names for version output.
var VersionFields = []string{"version", "commit", "goVersion", "os", "arch"}

// NewCmd creates the version command that displays build information.
// In human mode it prints a multi-line summary; with --json it outputs
// structured JSON with version, commit, goVersion, os, and arch fields.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show ytr version information",
		Long: `Display the version, commit hash, Go version, and platform of the ytr binary.

JSON FIELDS
  version, commit, goVersion, os, arch

SEE ALSO
  ytr --help    - Show all available commands`,
		Example: `  # Show version
  ytr version

  # Get version as JSON
  ytr version --json version,commit

  # Get just the version string
  ytr version --json version --jq '.version'`,
		Args: cobra.NoArgs,
		RunE: runVersion,
	}

	jsonfields.Register("ytr version", VersionFields)

	return cmd
}

// runVersion executes the version display logic.
func runVersion(cmd *cobra.Command, _ []string) error {
	info := ver.Get()

	// Handle --json field hint (no fields specified, D-10).
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "version", VersionFields)
	}

	// If --jq without --json: auto-populate all fields (Pitfall 5).
	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = VersionFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, VersionFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, VersionFields)
	}

	if output.IsJSON() {
		if output.HasFieldSelection() {
			filtered := output.FilterFields(info, output.JSONFields)
			if output.JQFilter != "" {
				return output.ApplyJQ(cmd.OutOrStdout(), filtered, output.JQFilter)
			}
			return output.PrintJSON(cmd.OutOrStdout(), filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(cmd.OutOrStdout(), info, output.JQFilter)
		}
		return output.PrintJSON(cmd.OutOrStdout(), info)
	}

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "ytr version %s\n", info.Version)
	_, _ = fmt.Fprintf(w, "commit: %s\n", info.Commit)
	_, _ = fmt.Fprintf(w, "go: %s\n", info.GoVersion)
	_, _ = fmt.Fprintf(w, "os/arch: %s/%s\n", info.OS, info.Arch)
	return nil
}
