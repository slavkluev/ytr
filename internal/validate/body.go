package validate

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/slavkluev/ytr/internal/errors"
)

// FromJSONFlag is the flag that gives a write command's request body whole,
// and FromJSONUsage its help on every command that takes it.
const (
	FromJSONFlag  = "from-json"
	FromJSONUsage = "Full JSON request body (inline, @file, or - for stdin)"
)

// Body is how a write command takes its request body: each of Flags sets one
// key, or --from-json gives the whole body.
type Body struct {
	// Flags are in declaration order, the order every error names them in.
	Flags []BodyFlag

	// Required are the keys the body cannot go without, in either form. Each
	// is the Key of one of Flags.
	Required []string

	// Update makes a body that sets no key an error.
	Update bool
}

// BodyFlag is a request flag, named without its dashes, and the body key it
// sets.
type BodyFlag struct {
	Name, Key string

	// Check, when set, refuses a value of Key, whether the flag or --from-json
	// gives it, with the same error either way.
	Check func(string) error
}

// CheckFlags checks the flags changed reports as set, before the --json check
// and auth: no request flag next to --from-json, and without --from-json
// every required flag, and on an update at least one flag.
func (b Body) CheckFlags(changed func(name string) bool) error {
	var set []string
	for _, f := range b.Flags {
		if changed(f.Name) {
			set = append(set, "--"+f.Name)
		}
	}

	if changed(FromJSONFlag) {
		if len(set) > 0 {
			return errors.NewUserError(
				"cannot combine --from-json with "+strings.Join(set, ", "),
				"Pass the request as flags or as --from-json, not both",
			)
		}

		return nil
	}

	if err := b.missing(func(f BodyFlag) bool { return !changed(f.Name) }); err != nil {
		return err
	}

	if b.Update && len(set) == 0 {
		return b.nothingToUpdate()
	}

	return nil
}

// Decode decodes data, the body --from-json or the flags give, into req with
// UnmarshalRequestJSON, and fails an update that would send no key, a body
// that would send no required key, and a value a flag's Check refuses.
func (b Body) Decode(data []byte, req any) error {
	if err := UnmarshalRequestJSON(data, req); err != nil {
		return err
	}

	// What req sends, not data, says whether a key is set: decoding matches a
	// key case-insensitively, and a null or empty value is left out of the
	// request.
	sent, err := json.Marshal(req) //nolint:forbidigo // read back for its keys, never written out
	if err != nil {
		return err
	}

	var present map[string]json.RawMessage
	if err := json.Unmarshal(sent, &present); err != nil {
		return err
	}

	if b.Update && len(present) == 0 {
		return b.nothingToUpdate()
	}

	if err := b.missing(func(f BodyFlag) bool {
		_, ok := present[f.Key]
		return !ok
	}); err != nil {
		return err
	}

	for _, f := range b.Flags {
		raw, ok := present[f.Key]
		if f.Check == nil || !ok {
			continue
		}

		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		if err := f.Check(value); err != nil {
			return err
		}
	}

	return nil
}

func (b Body) missing(absent func(BodyFlag) bool) error {
	var flags, keys []string
	for _, f := range b.Flags {
		if slices.Contains(b.Required, f.Key) && absent(f) {
			flags = append(flags, "--"+f.Name)
			keys = append(keys, strconv.Quote(f.Key))
		}
	}

	var suggestion string
	switch len(flags) {
	case 0:
		return nil
	case 1:
		suggestion = "Pass it as a flag, or as the key " + keys[0] + " in --from-json"
	default:
		suggestion = "Pass them as flags, or as the keys " + strings.Join(keys, ", ") + " in --from-json"
	}

	return errors.NewUserError("missing "+strings.Join(flags, ", "), suggestion)
}

func (b Body) nothingToUpdate() error {
	names := make([]string, len(b.Flags))
	for i, f := range b.Flags {
		names[i] = "--" + f.Name
	}

	suggestion := "Pass at least one of " + strings.Join(names, ", ")
	if len(names) == 1 {
		suggestion = "Pass " + names[0]
	}

	return errors.NewUserError(
		"nothing to update",
		suggestion+", or a --from-json object with at least one key",
	)
}
