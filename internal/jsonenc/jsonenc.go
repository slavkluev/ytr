// Package jsonenc encodes the JSON ytr writes for its reader.
//
// encoding/json escapes <, > and & as \u003c, \u003e and \u0026 so the
// result is safe inside an HTML page. ytr never writes to one: its reader is an
// agent or a terminal, Tracker text is Markdown full of code, HTML and &&, and
// each escape makes that text harder to read and costs the agent tokens, while
// any JSON parser decodes both forms to the same string.
//
// The encoder copies the bytes a MarshalJSON method returns as they are, so a
// type that renders its own JSON for ytr's output builds it with this package
// too, or its escapes survive.
package jsonenc

import (
	"bytes"
	"encoding/json"
)

// Marshal is json.Marshal that leaves <, > and & as they are.
func Marshal(v any) ([]byte, error) {
	return MarshalIndent(v, "", "")
}

// MarshalIndent is json.MarshalIndent that leaves <, > and & as they are.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent(prefix, indent)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	// Encode ends the document with a newline, which the json functions this
	// stands in for do not.
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}
