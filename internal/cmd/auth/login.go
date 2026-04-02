package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
)

// userValidator abstracts the Users.Myself call for testability.
type userValidator interface {
	Myself(ctx context.Context) (*tracker.User, *tracker.Response, error)
}

type orgTypeDetector interface {
	Detect(ctx context.Context, token, orgID string) (config.OrgType, *tracker.User, error)
}

// newValidator creates a userValidator from resolved auth credentials.
// Tests override this variable to inject mocks.
var newValidator = func(auth *config.ResolvedAuth) userValidator {
	return api.NewClient(auth).Users
}

type defaultOrgTypeDetector struct{}

type orgTypeAttemptFailure struct {
	OrgType config.OrgType
	Err     error
}

type orgTypeDetectionError struct {
	Failures []orgTypeAttemptFailure
}

func (e *orgTypeDetectionError) Error() string {
	if e == nil || len(e.Failures) == 0 {
		return "failed to detect organization type"
	}

	parts := make([]string, 0, len(e.Failures))
	for _, failure := range e.Failures {
		parts = append(parts, fmt.Sprintf("%s: %s", failure.OrgType, failure.Err.Error()))
	}

	return "failed to detect organization type: " + strings.Join(parts, "; ")
}

func (e *orgTypeDetectionError) Unwrap() []error {
	if e == nil || len(e.Failures) == 0 {
		return nil
	}

	errs := make([]error, 0, len(e.Failures))
	for _, failure := range e.Failures {
		if failure.Err != nil {
			errs = append(errs, failure.Err)
		}
	}

	return errs
}

func classifyOrgTypeDetectionError(err *orgTypeDetectionError) error {
	if err == nil {
		return ytrerrors.NewUserError(
			"failed to detect organization type",
			"Retry with --org-type 360 or --org-type cloud",
		)
	}

	code, ok := detectSharedSemanticCode(err.Failures)
	if !ok {
		if hasConflictingSemanticCodes(err.Failures) {
			return ytrerrors.NewUserError(
				err.Error(),
				"Retry with --org-type 360 or --org-type cloud",
			)
		}

		return ytrerrors.NewUserError(
			err.Error(),
			detectFailureSuggestion(err.Failures),
		)
	}

	switch code {
	case ytrerrors.CodeAuthError:
		return ytrerrors.NewAuthError(
			err.Error(),
			"Check your token and access to the Tracker organization",
		)
	case ytrerrors.CodeRateLimited:
		return ytrerrors.NewRateLimitedError(
			err.Error(),
			"Wait and retry",
		)
	case ytrerrors.CodeNotFound:
		return ytrerrors.NewNotFoundError(
			err.Error(),
			"Check your organization ID and access to the Tracker organization",
		)
	case ytrerrors.CodeUserError:
		return ytrerrors.NewUserError(
			err.Error(),
			detectFailureSuggestion(err.Failures),
		)
	default:
		return ytrerrors.NewUserError(
			err.Error(),
			detectFailureSuggestion(err.Failures),
		)
	}
}

func detectSharedSemanticCode(failures []orgTypeAttemptFailure) (string, bool) {
	var code string

	for _, failure := range failures {
		mappedErr := api.MapAPIError(failure.Err)
		exitErr := &ytrerrors.ExitError{}
		if !errors.As(mappedErr, &exitErr) {
			return "", false
		}

		if code == "" {
			code = exitErr.Code
			continue
		}
		if exitErr.Code != code {
			return "", false
		}
	}

	return code, code != ""
}

func hasConflictingSemanticCodes(failures []orgTypeAttemptFailure) bool {
	codes := make(map[string]struct{}, len(failures))

	for _, failure := range failures {
		mappedErr := api.MapAPIError(failure.Err)
		exitErr := &ytrerrors.ExitError{}
		if !errors.As(mappedErr, &exitErr) {
			return false
		}

		codes[exitErr.Code] = struct{}{}
	}

	return len(codes) > 1
}

func detectFailureSuggestion(failures []orgTypeAttemptFailure) string {
	switch {
	case allServerErrors(failures):
		return "Retry later"
	case allTransportFailures(failures):
		return "Check network connectivity and retry"
	default:
		return "Review the reported Tracker error details and retry"
	}
}

func allServerErrors(failures []orgTypeAttemptFailure) bool {
	if len(failures) == 0 {
		return false
	}

	for _, failure := range failures {
		var errResp *tracker.ErrorResponse
		if !errors.As(failure.Err, &errResp) ||
			errResp.Response == nil ||
			errResp.Response.StatusCode < 500 {
			return false
		}
	}

	return true
}

func allTransportFailures(failures []orgTypeAttemptFailure) bool {
	if len(failures) == 0 {
		return false
	}

	for _, failure := range failures {
		var errResp *tracker.ErrorResponse
		if errors.As(failure.Err, &errResp) {
			return false
		}

		mappedErr := api.MapAPIError(failure.Err)
		exitErr := &ytrerrors.ExitError{}
		if errors.As(mappedErr, &exitErr) {
			return false
		}
	}

	return true
}

func (d defaultOrgTypeDetector) Detect(
	ctx context.Context,
	token, orgID string,
) (config.OrgType, *tracker.User, error) {
	var failures []orgTypeAttemptFailure

	for _, orgType := range []config.OrgType{
		config.OrgType360,
		config.OrgTypeCloud,
	} {
		auth := &config.ResolvedAuth{
			Token:       token,
			OrgID:       orgID,
			OrgType:     orgType,
			TokenSource: "flag",
		}

		user, _, err := newValidator(auth).Myself(ctx)
		if err == nil {
			return orgType, user, nil
		}

		failures = append(failures, orgTypeAttemptFailure{
			OrgType: orgType,
			Err:     err,
		})
	}

	return "", nil, &orgTypeDetectionError{Failures: failures}
}

// detectOrgType tries supported Tracker organization modes and returns
// the first one that successfully validates via Users.Myself.
// Tests override this variable to avoid network access.
var detectOrgType orgTypeDetector = defaultOrgTypeDetector{}

// stdinFile is the file used for reading stdin input.
// Tests override this to inject piped input.
var stdinFile *os.File = os.Stdin

// newLoginCmd creates the "auth login" command for authenticating
// with Yandex Tracker. Supports interactive masked prompt, piped stdin,
// and explicit --token/--org-id/--org-type flags.
func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Yandex Tracker",
		Long: `Authenticate with Yandex Tracker interactively. Prompts for token and organization ID,
detects organization type when needed, validates credentials via API call,
and saves them to config file.

SEE ALSO
  ytr auth status   - Check authentication status
  ytr auth logout   - Remove stored credentials`,
		Example: `  # Interactive login
	  ytr auth login

  # Login with flags
  ytr auth login --token TOKEN --org-id ORG --org-type 360

  # Login via pipe
  echo TOKEN | ytr auth login --org-id ORG`,
		Args: cobra.NoArgs,
		RunE: runLogin,
	}

	cmd.Flags().String("token", "", "OAuth token for authentication")
	cmd.Flags().String("org-id", "", "Tracker organization ID")
	cmd.Flags().String("org-type", "", "Organization type (360 or cloud)")

	return cmd
}

func runLogin(cmd *cobra.Command, args []string) error {
	tokenFlag, _ := cmd.Flags().GetString("token")
	orgIDFlag, _ := cmd.Flags().GetString("org-id")
	orgTypeFlag, _ := cmd.Flags().GetString("org-type")

	// Step 1: Resolve token
	token, err := resolveToken(tokenFlag, cmd)
	if err != nil {
		return err
	}

	// Step 2: Resolve org-id
	orgID, err := resolveOrgID(orgIDFlag, cmd)
	if err != nil {
		return err
	}

	// Step 3: Resolve organization type and validate credentials via API.
	orgType, err := resolveOrgType(orgTypeFlag)
	if err != nil {
		return err
	}

	user, orgType, err := resolveUserAndOrgType(cmd, token, orgID, orgType)
	if err != nil {
		return err
	}

	// Step 4: Extract username
	username := api.DerefUser(user, "unknown")

	// Step 5: Save config
	if err := config.Save(&config.Config{
		Token:   token,
		OrgID:   orgID,
		OrgType: orgType,
	}); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Step 6: Output result
	// Auth commands use cmd.Flags().Changed("json") for JSON detection.
	// No field selection or hints -- fixed-structure JSON.
	// Config was just saved successfully, so ConfigFilePath cannot fail.
	cfgPath, _ := config.ConfigFilePath()

	jsonRequested := cmd.Flags().Changed("json") || output.IsJSON()
	if jsonRequested {
		return output.PrintJSON(cmd.OutOrStdout(), map[string]string{
			"status":      "authenticated",
			"user":        username,
			"org_id":      orgID,
			"org_type":    string(orgType),
			"config_path": cfgPath,
		})
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"Authenticated as %s (org: %s, type: %s)\nConfig saved to %s\n",
		username, orgID, orgType, cfgPath)
	return nil
}

// resolveUserAndOrgType detects or validates the org type and returns the
// authenticated user. When orgType is empty, auto-detection is attempted.
func resolveUserAndOrgType(
	cmd *cobra.Command, token, orgID string, orgType config.OrgType,
) (*tracker.User, config.OrgType, error) {
	if orgType == "" {
		detectedType, user, err := detectOrgType.Detect(cmd.Context(), token, orgID)
		if err != nil {
			var detectErr *orgTypeDetectionError
			if errors.As(err, &detectErr) {
				return nil, "", classifyOrgTypeDetectionError(detectErr)
			}
			return nil, "", api.MapAPIError(err)
		}
		return user, detectedType, nil
	}

	auth := &config.ResolvedAuth{
		Token:       token,
		OrgID:       orgID,
		OrgType:     orgType,
		TokenSource: "flag",
	}

	validator := newValidator(auth)
	user, _, err := validator.Myself(cmd.Context())
	if err != nil {
		return nil, "", api.MapAPIError(err)
	}

	return user, orgType, nil
}

// resolveToken returns the token from the flag, piped stdin, or
// interactive masked prompt. Returns an error if no token is available.
func resolveToken(flagValue string, cmd *cobra.Command) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	return readToken(stdinFile, cmd)
}

// readToken reads a token from the given file descriptor.
// If the file is a terminal, it prompts with masked input via term.ReadPassword.
// If the file is a pipe, it reads a single line via bufio.Scanner.
func readToken(stdin *os.File, cmd *cobra.Command) (string, error) {
	if isatty.IsTerminal(stdin.Fd()) || isatty.IsCygwinTerminal(stdin.Fd()) {
		// Interactive: masked prompt
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Token: ")
		//nolint:gosec // fd conversion is safe for terminal operations
		tokenBytes, err := term.ReadPassword(int(stdin.Fd()))
		_, _ = fmt.Fprintln(cmd.ErrOrStderr()) // newline after masked input
		if err != nil {
			return "", fmt.Errorf("failed to read token: %w", err)
		}
		token := strings.TrimSpace(string(tokenBytes))
		if token == "" {
			return "", ytrerrors.NewUserError(
				"token is required",
				"Provide a token: ytr auth login --token TOKEN",
			)
		}
		return token, nil
	}

	// Piped: read single line
	scanner := bufio.NewScanner(stdin)
	if scanner.Scan() {
		token := strings.TrimSpace(scanner.Text())
		if token == "" {
			return "", ytrerrors.NewUserError(
				"empty token from stdin",
				"Pipe a non-empty token: echo TOKEN | ytr auth login --org-id ORG",
			)
		}
		return token, nil
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read token from stdin: %w", err)
	}

	return "", ytrerrors.NewUserError(
		"no token provided",
		"Provide a token: ytr auth login --token TOKEN",
	)
}

// resolveOrgID returns the org-id from the flag or interactive prompt.
// Returns an error if no org-id is available.
func resolveOrgID(flagValue string, cmd *cobra.Command) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	// Check if stdin is a terminal for interactive prompt
	if isatty.IsTerminal(stdinFile.Fd()) || isatty.IsCygwinTerminal(stdinFile.Fd()) {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Organization ID: ")
		scanner := bufio.NewScanner(stdinFile)
		if scanner.Scan() {
			orgID := strings.TrimSpace(scanner.Text())
			if orgID != "" {
				return orgID, nil
			}
		}
	}

	return "", ytrerrors.NewUserError(
		"org-id is required",
		"Use --org-id flag: ytr auth login --org-id ORG",
	)
}

// resolveOrgType validates the optional --org-type flag.
// An empty value means the type should be auto-detected.
func resolveOrgType(flagValue string) (config.OrgType, error) {
	if flagValue == "" {
		return "", nil
	}

	orgType, err := config.ParseOrgType(flagValue)
	if err != nil {
		return "", ytrerrors.NewUserError(
			err.Error(),
			"Use --org-type 360 or --org-type cloud",
		)
	}

	return orgType, nil
}
