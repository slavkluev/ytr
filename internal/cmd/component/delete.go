package component

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete COMPONENT-ID",
		Short: "Delete a component",
		Long: `Delete a project component from Yandex Tracker.

SEE ALSO
  ytr component list    - List all components
  ytr component get     - Show component details
  ytr component create  - Create a component`,
		Example: `  # Delete component 42
  ytr component delete 42

  # Delete and confirm via JSON
  ytr component delete 42 --json id`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, args []string) error {
			_, err := validate.ValidateNumericID(args[0], "component ID")
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd, args[0])
		},
	}

	return cmd
}

func runDelete(cmd *cobra.Command, componentID string) error {
	opts := output.FromContext(cmd.Context())

	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	deleter := newComponentDeleter(auth)

	// API returns (*Response, error) -- no body on 204 No Content.
	_, err = deleter.Delete(cmd.Context(), componentID)
	if err != nil {
		return api.MapAPIError(err)
	}

	w := cmd.OutOrStdout()

	if opts.IsJSON() {
		result := map[string]any{"id": componentID, "deleted": true}
		if opts.JQFilter != "" {
			return output.ApplyJQ(w, result, opts.JQFilter)
		}
		return opts.PrintJSON(w, result)
	}

	if opts.Quiet {
		output.PrintQuiet(w, componentID)
		return nil
	}

	_, err = fmt.Fprintf(w, "Component %s deleted\n", componentID)
	return err
}
