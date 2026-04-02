// Package completion provides shell completion script generation for ytr.
package completion

import (
	"github.com/spf13/cobra"
)

// NewCmd creates the "completion" command with bash, zsh, and fish subcommands.
// rootCmd is needed to generate completions for the full command tree.
func NewCmd(rootCmd *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for ytr.

To load completions:

Bash:
  source <(ytr completion bash)

  # To load completions for each session, execute once:
  # Linux:
  ytr completion bash > /etc/bash_completion.d/ytr
  # macOS:
  ytr completion bash > $(brew --prefix)/etc/bash_completion.d/ytr

Zsh:
  source <(ytr completion zsh)

  # To load completions for each session, execute once:
  ytr completion zsh > "${fpath[1]}/_ytr"

Fish:
  ytr completion fish | source

  # To load completions for each session, execute once:
  ytr completion fish > ~/.config/fish/completions/ytr.fish

SEE ALSO
  ytr --help    - Show all available commands`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		Long:  "Generate bash completion script for ytr. Output to stdout.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenBashCompletionV2(cmd.OutOrStdout(), true)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		Long:  "Generate zsh completion script for ytr. Output to stdout.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenZshCompletion(cmd.OutOrStdout())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		Long:  "Generate fish completion script for ytr. Output to stdout.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenFishCompletion(cmd.OutOrStdout(), true)
		},
	})

	return cmd
}
