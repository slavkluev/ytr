// Package cmd provides the root Cobra command and command tree for ytr CLI.
package cmd

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd/auth"
	"github.com/slavkluev/ytr/internal/cmd/checklist"
	"github.com/slavkluev/ytr/internal/cmd/comment"
	"github.com/slavkluev/ytr/internal/cmd/field"
	"github.com/slavkluev/ytr/internal/cmd/issue"
	"github.com/slavkluev/ytr/internal/cmd/issuetype"
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

// usageTemplate is cobra's default usage template without the group titles,
// the Global Flags block and the closing footer. Root's persistent flags are
// its local flags, so root help lists them once, and every other page leaves
// them out instead of repeating them.
const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}
`

// Every call returns an independent tree. pflag writes a parsed value into the
// variable the flag was bound to and remembers that the flag was Changed, and
// SetArgs sticks to the command, so a tree that has already run would carry that
// state into the next run. The output flags are bound into opts, which the run
// must carry in its context for the commands to see them.
func newRootCmd(opts *output.Options) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "ytr",
		Short:         "Yandex Tracker CLI",
		Long:          "Command-line client for Yandex Tracker. Designed for LLM agents.",
		SilenceErrors: true,
		SilenceUsage:  true,
		// Cobra adds its own completion command on every Execute unless told
		// not to.
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}

	addPersistentFlags(rootCmd, opts)
	rootCmd.SetHelpCommand(newHelpCmd())
	rootCmd.SetUsageTemplate(usageTemplate)
	registerSubcommands(rootCmd)

	// Cobra adds the help command to the tree when it executes; doing it here
	// means the tree this returns is the tree that runs, so the contract below
	// covers the help command too.
	rootCmd.InitDefaultHelpCmd()

	rootCmd.SetFlagErrorFunc(flagError)
	installInvocationContract(rootCmd)
	hideDispatchOnlyUsageLine(rootCmd)

	return rootCmd
}

func addPersistentFlags(rootCmd *cobra.Command, opts *output.Options) {
	rootCmd.PersistentFlags().
		StringSliceVar(&opts.JSONFields, "json", nil, "Output JSON with selected fields (comma-separated)")
	rootCmd.PersistentFlags().
		StringVar(&opts.JQFilter, "jq", "", "Filter JSON output with a jq expression (implies --json)")
	rootCmd.PersistentFlags().
		BoolVar(&opts.Debug, "debug", false, "Emit sanitized debug diagnostics to stderr")
	// Hidden from help: the agents who read help have no use for it, but it
	// still parses for whoever diagnoses a run.
	_ = rootCmd.PersistentFlags().MarkHidden("debug")

	rootCmd.PersistentFlags().String("token", "", "Authentication token (use with --org-id and --org-type)")
	rootCmd.PersistentFlags().String("org-id", "", "Tracker organization ID (use with --token and --org-type)")
	rootCmd.PersistentFlags().String("org-type", "", "Organization type, 360 or cloud (use with --token and --org-id)")
}

func registerSubcommands(rootCmd *cobra.Command) {
	rootCmd.AddCommand(
		issue.NewCmd(),
		comment.NewCmd(),
		link.NewCmd(),
		worklog.NewCmd(),
		checklist.NewCmd(),
		status.NewCmd(),
		priority.NewCmd(),
		resolution.NewCmd(),
		issuetype.NewCmd(),
		field.NewCmd(),
		queue.NewCmd(),
		user.NewCmd(),
		auth.NewCmd(),
		versioncmd.NewCmd(),
	)
}

// Execute runs the root command and returns the appropriate exit code.
// The caller (main.go) must pass this to os.Exit.
func Execute() int {
	stdout, stderr := os.Stdout, os.Stderr //nolint:forbidigo // the process's streams are wired here
	return execute(context.Background(), os.Args[1:], os.Stdin, stdout, stderr)
}

// Debug diagnostics share errOut with the error document, so a caller reading
// stderr sees them in the order they happened.
func execute(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	// SetArgs(nil) makes cobra fall back to os.Args[1:], which would turn a bare
	// invocation into whatever the process was started with.
	if args == nil {
		args = []string{}
	}

	opts := output.Options{DebugOut: errOut}
	root := newRootCmd(&opts)
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)

	err := root.ExecuteContext(output.NewContext(ctx, &opts))

	return opts.HandleInvocationError(errOut, err)
}

// RootCmd returns a freshly built root command whose context carries the
// options its flags are bound to, so a command run on it sees its output flags.
// Each call returns an independent tree; see newRootCmd.
func RootCmd() *cobra.Command {
	opts := &output.Options{}
	root := newRootCmd(opts)
	root.SetContext(output.NewContext(context.Background(), opts))

	return root
}
