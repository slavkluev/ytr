package bulk

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const (
	initialBackoff = 1 * time.Second

	// The wait ends well before an agent's tool timeout, 2 minutes for
	// Claude Code's Bash tool, at which the harness kills the command before
	// it can print the operation ID. Polls keep coming through the last
	// stretch of it, so the state the wait ends with is recent.
	defaultTimeout = 60 * time.Second
	maxBackoff     = 10 * time.Second

	bulkStatusDone = "COMPLETED"
	bulkStatusFail = "FAILED"
)

func readIssueKeys(args []string, stdin io.Reader) ([]string, error) {
	if len(args) > 0 {
		return dedupeKeys(args), nil
	}

	var keys []string
	scanner := bufio.NewScanner(stdin)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		keys = append(keys, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read stdin: %w", err)
	}

	if len(keys) == 0 {
		return nil, ytrerrors.NewUserError(
			"no issue keys provided via stdin",
			"Pipe issue keys via stdin (one per line) or provide as arguments",
		)
	}

	return dedupeKeys(keys), nil
}

// Body.Decode checks only the keys flags set, and no flag sets "issues".
func checkIssues(issues []string) error {
	if len(issues) == 0 {
		return ytrerrors.NewUserError(
			"no issue keys provided",
			`Pass them as the key "issues" in --from-json`,
		)
	}

	for _, issue := range issues {
		if err := validate.ValidateIssueKeyOrID(issue); err != nil {
			return err
		}
	}

	return nil
}

// Bulk requests should not carry the same key twice (the API would process it
// redundantly), and duplicates commonly arrive when piping unsorted output.
func dedupeKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

// Splits on the first = sign only, so values may contain =.
func parseFieldFlags(fields []string) (map[string]any, error) {
	values := make(map[string]any, len(fields))

	for _, field := range fields {
		idx := strings.Index(field, "=")
		if idx < 1 {
			return nil, ytrerrors.NewUserError(
				fmt.Sprintf("invalid field format %q: expected key=value", field),
				"Use --field key=value (e.g., --field priority=critical)",
			)
		}

		key := field[:idx]
		val := field[idx+1:]
		values[key] = val
	}

	return values, nil
}

// errWaitEnded is what pollUntilDone returns when its context ends before the
// operation finishes. A poll's own HTTP timeout also matches
// context.DeadlineExceeded, so the end of the wait cannot be told by that.
var errWaitEnded = errors.New("the wait ended before the bulk operation finished")

func finished(status string) bool {
	return status == bulkStatusDone || status == bulkStatusFail
}

// pollUntilDone returns the operation start began once it has finished. When
// ctx ends first, it returns errWaitEnded with the last state Tracker
// reported: start itself if no poll had answered yet.
func pollUntilDone(
	ctx context.Context,
	getter *tracker.BulkChangeService,
	start *tracker.BulkChange,
	operationID string,
) (*tracker.BulkChange, error) {
	last, backoff := start, initialBackoff

	for {
		select {
		case <-ctx.Done():
			return last, errWaitEnded
		case <-time.After(backoff):
		}

		bc, _, err := getter.GetStatus(ctx, operationID)
		if err != nil && ctx.Err() != nil {
			return last, errWaitEnded
		}
		if err != nil {
			return nil, api.MapAPIError(err)
		}

		if finished(api.DerefString(bc.Status, "")) {
			return bc, nil
		}

		last = bc

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func awaitBulkCompletion(
	cmd *cobra.Command,
	opts *output.Options,
	getter *tracker.BulkChangeService,
	bc *tracker.BulkChange,
	timeout time.Duration,
) error {
	operationID := api.DerefFlexString(bc.ID, "")
	if operationID == "" {
		return &ytrerrors.ExitError{
			ExitCode: ytrerrors.ExitUserError,
			Code:     "bulk_no_operation_id",
			Message:  "bulk operation was created but the API returned no operation ID",
			Suggestion: "Retry the command; if it persists, verify the request and " +
				"check the operation in the Tracker UI",
		}
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	result, err := pollUntilDone(ctx, getter, bc, operationID)
	if err != nil && !errors.Is(err, errWaitEnded) {
		return err
	}

	return finalizeBulkResult(cmd, opts, result, operationID)
}

// A failed operation renders nothing: a run that ends non-zero must leave
// stdout empty, so a reader never has to decide whether the document it found
// there describes a change that happened. The counts the result carried travel
// in the error instead, and reach stderr with it. `bulk status` ends here too,
// so the exit code says whether the change happened whichever command found it
// FAILED.
//
// An operation still running when the wait ends renders like a finished one,
// at exit 0: Tracker has accepted the change, and a failure would invite a
// retry that starts a second operation. Its status says it has not finished,
// and its suggestion is the command that checks it again.
func finalizeBulkResult(cmd *cobra.Command, opts *output.Options, bc *tracker.BulkChange, operationID string) error {
	if api.DerefString(bc.Status, "") == bulkStatusFail {
		return ytrerrors.NewBulkFailedError(
			operationID,
			api.DerefString(bc.StatusText, ""),
			api.DerefInt(bc.TotalIssues, 0),
			api.DerefInt(bc.TotalCompletedIssues, 0),
		)
	}

	return renderBulkOutput(cmd, opts, bc)
}

func renderBulkOutput(cmd *cobra.Command, opts *output.Options, bc *tracker.BulkChange) error {
	return runner.PrintJSON(cmd, opts, output.FilterFields(toBulkChangeDetail(bc), opts.JSONFields))
}
