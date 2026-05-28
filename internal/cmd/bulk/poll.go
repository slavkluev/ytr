package bulk

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/output"
	"github.com/slavkluev/ytr/internal/validate"
)

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
	defaultTimeout = 5 * time.Minute
	bulkStatusDone = "COMPLETED"
	bulkStatusFail = "FAILED"
)

// stdinFile is the file used for reading stdin input.
// Tests override this to inject piped input.
var stdinFile *os.File = os.Stdin

// stderrFile is the file used for progress output.
// Tests override this to suppress TTY progress display.
var stderrFile *os.File = os.Stderr

// readIssueKeys reads issue keys from positional args or stdin pipe.
// Positional args take priority over stdin. Each key is validated
// via ValidateIssueKey. Blank lines in stdin are skipped.
func readIssueKeys(args []string) ([]string, error) {
	if len(args) > 0 {
		for _, key := range args {
			if err := validate.ValidateIssueKey(key); err != nil {
				return nil, err
			}
		}
		return dedupeKeys(args), nil
	}

	fd := stdinFile.Fd()
	if isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd) {
		return nil, ytrerrors.NewUserError(
			"no issue keys provided",
			"Provide keys as arguments or pipe them via stdin (one per line)",
		)
	}

	var keys []string
	scanner := bufio.NewScanner(stdinFile)

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

	for _, key := range keys {
		if err := validate.ValidateIssueKey(key); err != nil {
			return nil, err
		}
	}

	return dedupeKeys(keys), nil
}

// dedupeKeys removes duplicate issue keys while preserving first-seen order.
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

// parseFieldFlags parses --field key=value flags into a map.
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

// isStderrTTY checks whether stderr is connected to a terminal.
func isStderrTTY() bool {
	fd := stderrFile.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// showProgress displays an updating progress line on stderr.
// Only outputs when stderr is a TTY; silent for non-TTY (agents).
func showProgress(w io.Writer, bc *tracker.BulkChange) {
	if !isStderrTTY() {
		return
	}

	done := api.DerefInt(bc.TotalCompletedIssues, 0)
	total := api.DerefInt(bc.TotalIssues, 0)
	pct := api.DerefInt(bc.ExecutionIssuePercent, 0)

	fmt.Fprintf(w, "\r%-60s",
		fmt.Sprintf("Bulk operation: %d/%d issues (%d%%)", done, total, pct))
}

// clearProgress clears the progress line on stderr.
// Only outputs when stderr is a TTY.
func clearProgress(w io.Writer) {
	if !isStderrTTY() {
		return
	}

	fmt.Fprintf(w, "\r%-60s\r", "")
}

// pollUntilDone polls a bulk operation until it reaches a terminal status
// (COMPLETED or FAILED) using exponential backoff.
// Returns the final BulkChange or a context error on timeout.
func pollUntilDone(
	ctx context.Context,
	getter bulkStatusGetter,
	operationID string,
	stderr io.Writer,
) (*tracker.BulkChange, error) {
	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		bc, _, err := getter.GetStatus(ctx, operationID)
		if err != nil {
			return nil, api.MapAPIError(err)
		}

		showProgress(stderr, bc)

		status := api.DerefString(bc.Status, "")
		if status == bulkStatusDone || status == bulkStatusFail {
			clearProgress(stderr)
			return bc, nil
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// awaitBulkCompletion extracts the operation ID from a freshly-created bulk
// change, polls until the operation reaches a terminal state, renders the
// result, and surfaces a non-zero exit code when the operation FAILED.
//
// It is used by bulk move/update/transition. The empty-ID guard prevents
// polling the collection endpoint with no ID (which produced a misleading
// "operation ID: " message).
func awaitBulkCompletion(
	cmd *cobra.Command,
	getter bulkStatusGetter,
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

	result, err := pollUntilDone(ctx, getter, operationID, cmd.ErrOrStderr())
	if err != nil {
		return handlePollError(ctx, err, timeout, operationID)
	}

	return finalizeBulkResult(cmd, result, operationID)
}

// finalizeBulkResult renders a terminal BulkChange and returns a non-zero
// ExitError when the operation finished in the FAILED state, so the process
// exit code honors the "success == 0" contract relied on by scripts and agents.
// Output details are still rendered for both COMPLETED and FAILED.
func finalizeBulkResult(cmd *cobra.Command, bc *tracker.BulkChange, operationID string) error {
	if err := renderBulkOutput(cmd, bc); err != nil {
		return err
	}

	if api.DerefString(bc.Status, "") == bulkStatusFail {
		msg := fmt.Sprintf("bulk operation %s failed", operationID)
		if statusText := api.DerefString(bc.StatusText, ""); statusText != "" {
			msg = fmt.Sprintf("%s: %s", msg, statusText)
		}
		return &ytrerrors.ExitError{
			ExitCode: ytrerrors.ExitUserError,
			Code:     "bulk_failed",
			Message:  msg,
		}
	}

	return nil
}

// renderBulkOutput renders a BulkChange in the appropriate output mode.
// Used by bulk status and by mutation commands after polling completes.
func renderBulkOutput(cmd *cobra.Command, bc *tracker.BulkChange) error {
	w := cmd.OutOrStdout()

	if output.IsJSON() {
		item := toBulkChangeDetail(bc)
		if output.HasFieldSelection() {
			filtered := output.FilterFields(item, output.JSONFields)
			if output.JQFilter != "" {
				return output.ApplyJQ(w, filtered, output.JQFilter)
			}
			return output.PrintJSON(w, filtered)
		}
		if output.JQFilter != "" {
			return output.ApplyJQ(w, item, output.JQFilter)
		}
		return output.PrintJSON(w, item)
	}

	if output.IsQuiet() {
		output.PrintQuiet(w, api.DerefFlexString(bc.ID, ""))
		return nil
	}

	// Table output: ID, STATUS, TOTAL, DONE, PERCENT.
	tbl := output.NewTable(w)
	tbl.AddHeader("ID", "STATUS", "TOTAL", "DONE", "PERCENT")
	tbl.AddRow(
		api.DerefFlexString(bc.ID, "-"),
		api.DerefString(bc.Status, "-"),
		strconv.Itoa(api.DerefInt(bc.TotalIssues, 0)),
		strconv.Itoa(api.DerefInt(bc.TotalCompletedIssues, 0)),
		fmt.Sprintf("%d%%", api.DerefInt(bc.ExecutionIssuePercent, 0)),
	)
	tbl.Render()

	return nil
}
