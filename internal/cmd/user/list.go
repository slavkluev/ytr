package user

import (
	"fmt"
	"io"
	"strconv"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const (
	defaultLimit = 50
	maxLimit     = 1000
)

// UserListFields lists the available JSON field names for user list output.
var UserListFields = []string{"uid", "display", "login", "email"}

type userItem struct {
	UID     int    `json:"uid"`
	Display string `json:"display"`
	Login   string `json:"login"`
	Email   string `json:"email,omitempty"`
}

func toUserItem(u *tracker.User) userItem {
	return userItem{
		UID:     api.DerefInt(u.UID, 0),
		Display: api.DerefString(u.Display, ""),
		Login:   api.DerefString(u.Login, ""),
		Email:   api.DerefString(u.Email, ""),
	}
}

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
  uid, display, login, email`,
		Example: `  # List all users
  ytr user list

  # List users as JSON
  ytr user list --json uid,display,login,email

  # Get all user logins
  ytr user list --all --json login --jq '.items[].login'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, limit, cursor, all)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "Maximum number of results per page (max 1000)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Page number for pagination")
	cmd.Flags().BoolVar(&all, "all", false, "Fetch all pages automatically")

	runner.SetFields(cmd, UserListFields)

	return cmd
}

type userSearchResult struct {
	users      []*tracker.User
	totalCount int
	hasMore    bool
	nextCursor string
}

func runList(cmd *cobra.Command, limit int, cursor string, all bool) error {
	opts := output.FromContext(cmd.Context())

	if opts.WantsFieldHint(cmd.Flags().Changed("json")) {
		return output.PrintFieldHint(cmd.ErrOrStderr(), "user list", UserListFields)
	}

	if err := validate.ConflictingAllAndCursor(
		cmd.Flags().Changed("all"), cmd.Flags().Changed("cursor"),
	); err != nil {
		return err
	}

	if opts.JQFilter != "" && !opts.HasFieldSelection() {
		opts.JSONFields = UserListFields
	}

	if opts.HasFieldSelection() {
		if err := output.ValidateFields(opts.JSONFields, UserListFields); err != nil {
			return err
		}
		opts.JSONFields = output.NormalizeFields(opts.JSONFields, UserListFields)
	}

	client, err := runner.Client(cmd)
	if err != nil {
		return err
	}

	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	page, err := validate.ParsePageCursor(cursor)
	if err != nil {
		return err
	}

	result, err := fetchUsers(cmd, client.Users, limit, page, all)
	if err != nil {
		return err
	}

	return renderListOutput(cmd.OutOrStdout(), opts, result)
}

func fetchUsers(cmd *cobra.Command, lister *tracker.UsersService, limit, page int,
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

func fetchAllUserPages(cmd *cobra.Command, lister *tracker.UsersService,
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

func renderListOutput(w io.Writer, opts *output.Options, result *userSearchResult) error {
	if opts.IsJSON() {
		return renderListJSON(w, opts, result)
	}

	if opts.Quiet {
		uids := make([]string, len(result.users))
		for i, u := range result.users {
			uids[i] = strconv.Itoa(api.DerefInt(u.UID, 0))
		}
		output.PrintQuiet(w, uids...)
		return nil
	}

	return renderListTable(w, opts, result.users)
}

func renderListJSON(w io.Writer, opts *output.Options, result *userSearchResult) error {
	items := make([]userItem, len(result.users))
	for i, u := range result.users {
		items[i] = toUserItem(u)
	}

	var data any
	if opts.HasFieldSelection() {
		filtered := make([]map[string]any, len(items))
		for i, item := range items {
			filtered[i] = output.FilterFields(item, opts.JSONFields)
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

	if opts.JQFilter != "" {
		return output.ApplyJQ(w, data, opts.JQFilter)
	}
	return opts.PrintJSON(w, data)
}

func renderListTable(w io.Writer, opts *output.Options, users []*tracker.User) error {
	if len(users) == 0 {
		_, printErr := fmt.Fprintln(w, "No users found")
		return printErr
	}

	tbl := opts.NewTable(w)
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
