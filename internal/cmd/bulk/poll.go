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
// Positional args take priority over stdin (D-01). Each key is validated
// via ValidateIssueKey (D-02). Blank lines in stdin are skipped.
func readIssueKeys(args []string) ([]string, error) {
	if len(args) > 0 {
		for _, key := range args {
			if err := validate.ValidateIssueKey(key); err != nil {
				return nil, err
			}
		}
		return args, nil
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

	return keys, nil
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

// showProgress displays an updating progress line on stderr (D-06).
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

// clearProgress clears the progress line on stderr (D-06).
// Only outputs when stderr is a TTY.
func clearProgress(w io.Writer) {
	if !isStderrTTY() {
		return
	}

	fmt.Fprintf(w, "\r%-60s\r", "")
}

// pollUntilDone polls a bulk operation until it reaches a terminal status
// (COMPLETED or FAILED) using exponential backoff (D-03, D-04, D-05).
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

// renderBulkOutput renders a BulkChange in the appropriate output mode (D-07).
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
