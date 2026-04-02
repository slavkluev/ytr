// Package cmd provides the root Cobra command and command tree for ytr CLI.
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/auth"
	"github.com/slavkluev/ytr/internal/cmd/issue"
	"github.com/slavkluev/ytr/internal/cmd/comment"
	"github.com/slavkluev/ytr/internal/cmd/link"
	"github.com/slavkluev/ytr/internal/cmd/queue"
	"github.com/slavkluev/ytr/internal/cmd/completion"
	"github.com/slavkluev/ytr/internal/cmd/status"
	"github.com/slavkluev/ytr/internal/cmd/priority"
	"github.com/slavkluev/ytr/internal/cmd/resolution"
	"github.com/slavkluev/ytr/internal/cmd/issuetype"
	"github.com/slavkluev/ytr/internal/cmd/field"
	"github.com/slavkluev/ytr/internal/cmd/user"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	versioncmd "github.com/slavkluev/ytr/internal/cmd/version"
	"github.com/slavkluev/ytr/internal/output"
)

// Command group IDs per D-01, D-02.
const (
	groupIssueTracking = "issue-tracking"
	groupReferenceData = "reference-data"
	groupOrganization  = "organization"
	groupAccount       = "account"
	groupSystem        = "system"
)

var rootCmd = &cobra.Command{
	Use:           "ytr",
	Short:         "Yandex Tracker CLI",
	Long:          "Command-line client for Yandex Tracker. Designed for LLM agents and human developers.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.PersistentFlags().
		StringSliceVar(&output.JSONFields, "json", nil, "Output JSON with selected fields (comma-separated)")
	rootCmd.PersistentFlags().
		StringVar(&output.JQFilter, "jq", "", "Filter JSON output with a jq expression (implies --json)")
	rootCmd.PersistentFlags().
		BoolVar(&output.QuietFlag, "quiet", false, "Output only primary identifiers, one per line")
	rootCmd.PersistentFlags().
		BoolVar(&output.DebugFlag, "debug", false, "Emit sanitized debug diagnostics to stderr")

	// D-04: Global auth flags for flag-based auth override on all commands
	rootCmd.PersistentFlags().String("token", "", "Authentication token (use with --org-id and --org-type)")
	rootCmd.PersistentFlags().String("org-id", "", "Tracker organization ID (use with --token and --org-type)")
	rootCmd.PersistentFlags().String("org-type", "", "Organization type, 360 or cloud (use with --token and --org-id)")

	// D-07: --json and --quiet are mutually exclusive
	rootCmd.MarkFlagsMutuallyExclusive("json", "quiet")
	rootCmd.MarkFlagsMutuallyExclusive("jq", "quiet")

	// D-01, D-02: Command groups by domain, Issue Tracking first, System last.
	rootCmd.AddGroup(
		&cobra.Group{ID: groupIssueTracking, Title: "Issue Tracking:"},
		&cobra.Group{ID: groupReferenceData, Title: "Reference Data:"},
		&cobra.Group{ID: groupOrganization, Title: "Organization:"},
		&cobra.Group{ID: groupAccount, Title: "Account:"},
		&cobra.Group{ID: groupSystem, Title: "System:"},
	)
	rootCmd.SetHelpCommandGroupID(groupSystem)

	registerSubcommands()

	// D-08/D-09/D-10: Single completion function delegates to jsonfields registry.
	_ = rootCmd.RegisterFlagCompletionFunc("json",
		func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			if fields, ok := jsonfields.Get(cmd.CommandPath()); ok {
				return fields, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	)
}

// addGroupedCommand creates a subcommand, sets its group, and adds it to rootCmd.
func addGroupedCommand(cmd *cobra.Command, groupID string) {
	cmd.GroupID = groupID
	rootCmd.AddCommand(cmd)
}

// registerSubcommands adds all subcommands to the root command with GroupID per D-01.
func registerSubcommands() {
	// System group.
	addGroupedCommand(versioncmd.NewCmd(), groupSystem)

	addGroupedCommand(auth.NewCmd(), groupAccount)
	addGroupedCommand(issue.NewCmd(), groupIssueTracking)
	addGroupedCommand(comment.NewCmd(), groupIssueTracking)
	addGroupedCommand(link.NewCmd(), groupIssueTracking)
	addGroupedCommand(queue.NewCmd(), groupOrganization)
	addGroupedCommand(completion.NewCmd(rootCmd), groupSystem)
	addGroupedCommand(status.NewCmd(), groupReferenceData)
	addGroupedCommand(priority.NewCmd(), groupReferenceData)
	addGroupedCommand(resolution.NewCmd(), groupReferenceData)
	addGroupedCommand(issuetype.NewCmd(), groupReferenceData)
	addGroupedCommand(field.NewCmd(), groupReferenceData)
	addGroupedCommand(user.NewCmd(), groupAccount)
}

// Execute runs the root command and returns the appropriate exit code.
// The caller (main.go) must pass this to os.Exit.
func Execute() int {
	err := rootCmd.Execute()
	return output.HandleError(os.Stderr, err)
}

// AddCommand registers subcommands on the root command.
// This allows subcommand packages to register themselves without
// accessing rootCmd directly.
func AddCommand(cmds ...*cobra.Command) {
	rootCmd.AddCommand(cmds...)
}

// RootCmd returns the root command for testing purposes.
func RootCmd() *cobra.Command {
	return rootCmd
}
