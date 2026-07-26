package user

import (
	"fmt"
	"io"
	"strconv"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/jsonfields"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const (
	defaultLimit = 50
	maxLimit     = 1000
)

// UserListFields lists the available JSON field names for user list output.
var UserListFields = []string{"uid", "display", "login", "email"}

// userItem is a clean struct for JSON serialization of user list items.
type userItem struct {
	UID     int    `json:"uid"`
	Display string `json:"display"`
	Login   string `json:"login"`
	Email   string `json:"email,omitempty"`
}

// toUserItem converts a tracker.User to a JSON-serializable list item.
func toUserItem(u *tracker.User) userItem {
	return userItem{
		UID:     api.DerefInt(u.UID, 0),
		Display: api.DerefString(u.Display, ""),
		Login:   api.DerefString(u.Login, ""),
		Email:   api.DerefString(u.Email, ""),
	}
}

// newListCmd creates the "user list" command with pagination.
func newListCmd() *cobra.Command {
	var (
		limit  int
		cursor string
		all    bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List organization users",
		Long: `List Yandex Tracker organization users with pagination.

JSON FIELDS
  uid, display, login, email

SEE ALSO
  ytr user myself   - Show current user
  ytr user get      - Show user details by UID`,
		Example: `  # List all users
  ytr user list

  # List users as JSON
  ytr user list --json uid,display,login,email

  # Get all user logins
  ytr user list --all --json login --jq '.items[].login'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, limit, cursor, all)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of results per page (max 1000)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Page number for pagination")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages automatically")

	jsonfields.Register("ytr user list", UserListFields)

	return cmd
}

// userSearchResult holds the pagination state from a list operation.
type userSearchResult struct {
	users      []*tracker.User
	totalCount int
	hasMore    bool
	nextCursor string
}

// runList executes the user list logic.
func runList(cmd *cobra.Command, limit int, cursor string, all bool) error {
	if output.IsJSON() && !output.HasFieldSelection() && output.JQFilter == "" {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "user list", UserListFields)
	}

	if output.JQFilter != "" && !output.HasFieldSelection() {
		output.JSONFields = UserListFields
	}

	// Validate requested fields.
	if output.HasFieldSelection() {
		if err := output.ValidateFields(output.JSONFields, UserListFields); err != nil {
			return err
		}
		output.JSONFields = output.NormalizeFields(output.JSONFields, UserListFields)
	}

	// Resolve auth from root persistent flags.
	tokenFlag, _ := cmd.Root().PersistentFlags().GetString("token")
	orgIDFlag, _ := cmd.Root().PersistentFlags().GetString("org-id")
	orgTypeFlag, _ := cmd.Root().PersistentFlags().GetString("org-type")

	auth, err := config.ResolveAuth(tokenFlag, orgIDFlag, orgTypeFlag)
	if err != nil {
		return err
	}

	lister := newUserLister(auth)

	// Validate and cap limit.
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	// Parse cursor as page number.
	page, err := validate.ParsePageCursor(cursor)
	if err != nil {
		return err
	}

	result, err := fetchUsers(cmd, lister, limit, page, all)
	if err != nil {
		return err
	}

	return renderListOutput(cmd.OutOrStdout(), result)
}

// fetchUsers retrieves users with optional auto-pagination.
func fetchUsers(cmd *cobra.Command, lister userLister, limit, page int,
	all bool) (*userSearchResult, error) {
	if all {
		return fetchAllUserPages(cmd, lister, limit)
	}

	opts := &tracker.UserListOptions{}
	opts.Page = page
	opts.PerPage = limit

	users, resp, listErr := lister.List(cmd.Context(), opts)
	if listErr != nil {
		return nil, api.MapAPIError(listErr)
	}

	result := &userSearchResult{users: users}
	if resp != nil {
		result.totalCount = resp.TotalCount
	}
	result.hasMore = len(users) == limit
	if result.hasMore {
		result.nextCursor = strconv.Itoa(page + 1)
	}
	return result, nil
}

// fetchAllUserPages auto-paginates through all user pages.
func fetchAllUserPages(cmd *cobra.Command, lister userLister,
	limit int) (*userSearchResult, error) {
	var allUsers []*tracker.User
	var totalCount int

	currentPage := 1
	for {
		opts := &tracker.UserListOptions{}
		opts.Page = currentPage
		opts.PerPage = limit

		users, resp, listErr := lister.List(cmd.Context(), opts)
		if listErr != nil {
			return nil, api.MapAPIError(listErr)
		}

		allUsers = append(allUsers, users...)
		if resp != nil {
			totalCount = resp.TotalCount
		}

		if len(users) < limit {
			break
		}
		currentPage++
	}

	return &userSearchResult{users: allUsers, totalCount: totalCount}, nil
}

// renderListOutput renders the user list in JSON, quiet, or table mode.
func renderListOutput(w io.Writer, result *userSearchResult) error {
	if output.IsJSON() {
		return renderListJSON(w, result)
	}

	if output.IsQuiet() {
		uids := make([]string, len(result.users))
		for i, u := range result.users {
			uids[i] = strconv.Itoa(api.DerefInt(u.UID, 0))
		}
		output.PrintQuiet(w, uids...)
		return nil
	}

	return renderListTable(w, result.users)
}

// renderListJSON renders the user list as paginated JSON.
func renderListJSON(w io.Writer, result *userSearchResult) error {
	items := make([]userItem, len(result.users))
	for i, u := range result.users {
		items[i] = toUserItem(u)
	}

	var data any
	if output.HasFieldSelection() {
		filtered := make([]map[string]any, len(items))
		for i, item := range items {
			filtered[i] = output.FilterFields(item, output.JSONFields)
		}
		data = output.PaginatedResult{
			Items: filtered,
			Pagination: output.PaginationMeta{
				Cursor: result.nextCursor, HasMore: result.hasMore, Total: result.totalCount,
			},
		}
	} else {
		data = output.PaginatedResult{
			Items: items,
			Pagination: output.PaginationMeta{
				Cursor: result.nextCursor, HasMore: result.hasMore, Total: result.totalCount,
			},
		}
	}

	if output.JQFilter != "" {
		return output.ApplyJQ(w, data, output.JQFilter)
	}
	return output.PrintJSON(w, data)
}

// renderListTable renders the user list as a formatted table.
func renderListTable(w io.Writer, users []*tracker.User) error {
	if len(users) == 0 {
		_, printErr := fmt.Fprintln(w, "No users found")
		return printErr
	}

	tbl := output.NewTable(w)
	tbl.AddHeader("UID", "DISPLAY", "LOGIN", "EMAIL")

	for _, u := range users {
		uid := strconv.Itoa(api.DerefInt(u.UID, 0))
		display := api.DerefString(u.Display, "-")
		login := api.DerefString(u.Login, "-")
		email := api.DerefString(u.Email, "-")

		tbl.AddRow(uid, display, login, email)
	}

	tbl.Render()
	return nil
}
