// Package cmd provides the root Cobra command and command tree for ytr CLI.
package cmd

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/auth"
	"github.com/slavkluev/ytr/internal/cmd/bulk"
	"github.com/slavkluev/ytr/internal/cmd/checklist"
	"github.com/slavkluev/ytr/internal/cmd/comment"
	"github.com/slavkluev/ytr/internal/cmd/completion"
	"github.com/slavkluev/ytr/internal/cmd/component"
	"github.com/slavkluev/ytr/internal/cmd/field"
	"github.com/slavkluev/ytr/internal/cmd/issue"
	"github.com/slavkluev/ytr/internal/cmd/issuetype"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/cmd/link"
	"github.com/slavkluev/ytr/internal/cmd/priority"
	"github.com/slavkluev/ytr/internal/cmd/queue"
	"github.com/slavkluev/ytr/internal/cmd/resolution"
	"github.com/slavkluev/ytr/internal/cmd/status"
	"github.com/slavkluev/ytr/internal/cmd/user"
	versioncmd "github.com/slavkluev/ytr/internal/cmd/version"
	"github.com/slavkluev/ytr/internal/cmd/worklog"
	"github.com/slavkluev/ytr/internal/output"
)

// Command group IDs.
const (
	groupIssueTracking = "issue-tracking"
	groupReferenceData = "reference-data"
	groupOrganization  = "organization"
	groupAccount       = "account"
	groupSystem        = "system"
)

// newRootCmd builds a complete command tree, contract included.
//
// Every call returns an independent tree. pflag writes a parsed value into the
// variable the flag was bound to and remembers that the flag was Changed, and
// SetArgs sticks to the command, so a tree that has already run would carry that
// state into the next run.
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "ytr",
		Short:         "Yandex Tracker CLI",
		Long:          "Command-line client for Yandex Tracker. Designed for LLM agents and human developers.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	addPersistentFlags(rootCmd)
	addCommandGroups(rootCmd)

	// Must precede SetHelpCommandGroupID, which only reaches a help command
	// that is already installed.
	rootCmd.SetHelpCommand(newHelpCmd())
	rootCmd.SetHelpCommandGroupID(groupSystem)

	registerSubcommands(rootCmd)

	// Single completion function delegates to jsonfields registry.
	_ = rootCmd.RegisterFlagCompletionFunc("json",
		func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			if fields, ok := jsonfields.Get(cmd.CommandPath()); ok {
				return fields, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	)

	// Cobra adds the help command to the tree when it executes; doing it here
	// means the tree this returns is the tree that runs, so the contract below
	// covers the help command too.
	rootCmd.InitDefaultHelpCmd()

	rootCmd.SetFlagErrorFunc(flagError)
	installInvocationContract(rootCmd)
	hideDispatchOnlyUsageLine(rootCmd)

	return rootCmd
}

// addPersistentFlags registers the global flags every command inherits.
func addPersistentFlags(rootCmd *cobra.Command) {
	rootCmd.PersistentFlags().
		StringSliceVar(&output.JSONFields, "json", nil, "Output JSON with selected fields (comma-separated)")
	rootCmd.PersistentFlags().
		StringVar(&output.JQFilter, "jq", "", "Filter JSON output with a jq expression (implies --json)")
	rootCmd.PersistentFlags().
		BoolVar(&output.QuietFlag, "quiet", false, "Output minimal text, one item per line")
	rootCmd.PersistentFlags().
		BoolVar(&output.DebugFlag, "debug", false, "Emit sanitized debug diagnostics to stderr")

	// Global auth flags for flag-based auth override on all commands.
	rootCmd.PersistentFlags().String("token", "", "Authentication token (use with --org-id and --org-type)")
	rootCmd.PersistentFlags().String("org-id", "", "Tracker organization ID (use with --token and --org-type)")
	rootCmd.PersistentFlags().String("org-type", "", "Organization type, 360 or cloud (use with --token and --org-id)")

	// --json and --quiet are mutually exclusive.
	rootCmd.MarkFlagsMutuallyExclusive("json", "quiet")
	rootCmd.MarkFlagsMutuallyExclusive("jq", "quiet")
}

// addCommandGroups declares the help groups subcommands are sorted into.
func addCommandGroups(rootCmd *cobra.Command) {
	rootCmd.AddGroup(
		&cobra.Group{ID: groupIssueTracking, Title: "Issue Tracking:"},
		&cobra.Group{ID: groupReferenceData, Title: "Reference Data:"},
		&cobra.Group{ID: groupOrganization, Title: "Organization:"},
		&cobra.Group{ID: groupAccount, Title: "Account:"},
		&cobra.Group{ID: groupSystem, Title: "System:"},
	)
}

// addGroupedCommand sets a subcommand's group and adds it to rootCmd.
func addGroupedCommand(rootCmd, cmd *cobra.Command, groupID string) {
	cmd.GroupID = groupID
	rootCmd.AddCommand(cmd)
}

// registerSubcommands adds all subcommands to the root command.
func registerSubcommands(rootCmd *cobra.Command) {
	// Issue Tracking group.
	addGroupedCommand(rootCmd, issue.NewCmd(), groupIssueTracking)
	addGroupedCommand(rootCmd, comment.NewCmd(), groupIssueTracking)
	addGroupedCommand(rootCmd, link.NewCmd(), groupIssueTracking)
	addGroupedCommand(rootCmd, worklog.NewCmd(), groupIssueTracking)
	addGroupedCommand(rootCmd, checklist.NewCmd(), groupIssueTracking)
	addGroupedCommand(rootCmd, bulk.NewCmd(), groupIssueTracking)

	// Reference Data group.
	addGroupedCommand(rootCmd, status.NewCmd(), groupReferenceData)
	addGroupedCommand(rootCmd, priority.NewCmd(), groupReferenceData)
	addGroupedCommand(rootCmd, resolution.NewCmd(), groupReferenceData)
	addGroupedCommand(rootCmd, issuetype.NewCmd(), groupReferenceData)
	addGroupedCommand(rootCmd, field.NewCmd(), groupReferenceData)

	// Organization group.
	addGroupedCommand(rootCmd, queue.NewCmd(), groupOrganization)
	addGroupedCommand(rootCmd, component.NewCmd(), groupOrganization)

	// Account group.
	addGroupedCommand(rootCmd, user.NewCmd(), groupAccount)
	addGroupedCommand(rootCmd, auth.NewCmd(), groupAccount)

	// System group.
	addGroupedCommand(rootCmd, versioncmd.NewCmd(), groupSystem)

	addGroupedCommand(rootCmd, completion.NewCmd(rootCmd), groupSystem)
}

// Execute runs the root command and returns the appropriate exit code.
// The caller (main.go) must pass this to os.Exit.
func Execute() int {
	return execute(newRootCmd(), os.Args[1:], os.Stdout, os.Stderr)
}

// execute runs root against args and renders whatever it returns.
//
// The arguments reach the renderer as well as cobra, because a failed
// invocation can hide the output mode it asked for: pflag stops at the first
// flag it does not know, so a --json placed after the mistake never lands in
// the globals IsJSON reads.
func execute(root *cobra.Command, args []string, out, errOut io.Writer) int {
	// SetArgs(nil) makes cobra fall back to os.Args[1:], which would turn a bare
	// invocation into whatever the process was started with.
	if args == nil {
		args = []string{}
	}

	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(errOut)

	return output.HandleInvocationError(errOut, root.Execute(), args)
}

// RootCmd returns a freshly built root command for testing purposes.
// Each call returns an independent tree; see newRootCmd.
func RootCmd() *cobra.Command {
	return newRootCmd()
}
