package bulk

import (
	"context"
	"encoding/json"

	"github.com/slavkluev/go-yandex-tracker/tracker"
	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/cmd/runner"
	"github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/validate"
)

// change is a bulk change that sends Tracker a Req, built from the issue keys
// and the request flags or given whole by --from-json, and waits for the
// operation Tracker starts to finish.
type change[Req any] struct {
	body   validate.Body
	issues func(*Req) []string
	start  func(*tracker.BulkChangeService, context.Context, *Req) (*tracker.BulkChange, *tracker.Response, error)
}

var fieldFlag = validate.BodyFlag{Name: "field", Key: "values"}

// command gives cmd, which brings its help, what every bulk change shares:
// issue keys as arguments, the flag checks, the run, and the --field,
// --from-json and --timeout flags. The caller adds the flags of its other body
// keys.
func (c change[Req]) command(cmd *cobra.Command) *cobra.Command {
	cmd.Args = cobra.ArbitraryArgs
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return c.body.CheckFlags(cmd.Flags().Changed)
	}
	cmd.RunE = c.run

	cmd.Flags().StringArray(fieldFlag.Name, nil, "Field to update (key=value, repeatable)")
	cmd.Flags().String(validate.FromJSONFlag, "", "Full JSON request body (inline, @file, or - for stdin)")
	cmd.Flags().Duration("timeout", defaultTimeout, "Maximum time to wait for the operation to finish")

	runner.SetFields(cmd, BulkStatusFields)

	return cmd
}

func (c change[Req]) run(cmd *cobra.Command, args []string) error {
	var req Req
	if err := c.flagRequest(cmd, args, &req); err != nil {
		return err
	}

	opts, err := runner.SelectFields(cmd, BulkStatusFields)
	if err != nil {
		return err
	}

	client, err := runner.Client(cmd)
	if err != nil {
		return err
	}

	if err = c.jsonRequest(cmd, &req); err != nil {
		return err
	}

	bc, _, err := c.start(client.BulkChange, cmd.Context(), &req)
	if err != nil {
		return api.MapAPIError(err)
	}

	timeout, _ := cmd.Flags().GetDuration("timeout")

	return awaitBulkCompletion(cmd, opts, client.BulkChange, bc, timeout)
}

// flagRequest decodes into req the body the issue keys and the request flags
// give, one key per flag, before the field hint and auth. It leaves req to
// jsonRequest when --from-json gives the body, whose "issues" stdin cannot add
// to: stdin may be the body itself.
func (c change[Req]) flagRequest(cmd *cobra.Command, args []string, req *Req) error {
	set := cmd.Flags()
	if set.Changed(validate.FromJSONFlag) {
		if len(args) > 0 {
			return errors.NewUserError(
				"cannot combine --from-json with issue keys",
				`Pass the issue keys as arguments or as the key "issues" in --from-json, not both`,
			)
		}

		return nil
	}

	keys, err := readIssueKeys(args, cmd.InOrStdin())
	if err != nil {
		return err
	}

	body := map[string]any{"issues": keys}
	for _, f := range c.body.Flags {
		if !set.Changed(f.Name) {
			continue
		}

		if f.Name != fieldFlag.Name {
			body[f.Key], _ = set.GetString(f.Name)
			continue
		}

		fields, _ := set.GetStringArray(f.Name)
		if body[f.Key], err = parseFieldFlags(fields); err != nil {
			return err
		}
	}

	data, err := json.Marshal(body) //nolint:forbidigo // decoded straight back into req, never written out
	if err != nil {
		return err
	}

	return c.decode(data, req)
}

// jsonRequest decodes the body --from-json gives into req through the same
// decoder as the flags, read only once auth has resolved.
func (c change[Req]) jsonRequest(cmd *cobra.Command, req *Req) error {
	set := cmd.Flags()
	if !set.Changed(validate.FromJSONFlag) {
		return nil
	}

	value, _ := set.GetString(validate.FromJSONFlag)
	data, err := validate.ParseJSONInputFrom(value, cmd.InOrStdin())
	if err != nil {
		return err
	}

	return c.decode(data, req)
}

func (c change[Req]) decode(data []byte, req *Req) error {
	if err := c.body.Decode(data, req); err != nil {
		return err
	}

	return checkIssues(c.issues(req))
}
