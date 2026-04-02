package output

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// DebugFlag controls whether sanitized diagnostics are emitted to stderr.
// Set by the --debug global persistent flag.
var DebugFlag bool

var debugWriter io.Writer = os.Stderr
var jsonErrorWriter io.Writer = os.Stdout

var debugStringRedactors = []struct {
	pattern *regexp.Regexp
	replace string
}{
	{
		pattern: regexp.MustCompile(
			`(?i)\bAuthorization\b([^A-Za-z0-9]+)` +
				`(Bearer|OAuth)\s+[A-Za-z0-9._~+/=-]+`,
		),
		replace: "Authorization$1$2 <redacted>",
	},
	{
		pattern: regexp.MustCompile(
			`(?i)\b(Bearer|OAuth)\s+[A-Za-z0-9._~+/=-]+`,
		),
		replace: "$1 <redacted>",
	},
	{
		pattern: regexp.MustCompile(
			`(?i)\b(token|access[_-]?token|refresh[_-]?token|password|secret|` +
				`signature|sig|api[_-]?key|cookie|session(?:id)?)\b` +
				`([^A-Za-z0-9]+)([A-Za-z0-9._~+/=-]{4,})`,
		),
		replace: "$1$2<redacted>",
	},
	{
		pattern: regexp.MustCompile(
			`(?i)((?:token|access[_-]?token|refresh[_-]?token|password|secret|` +
				`signature|sig|api[_-]?key|cookie|session(?:id)?)["']?\s*[:=]\s*["']?)` +
				`([^"',\s&;]+)`,
		),
		replace: "$1<redacted>",
	},
}

// DebugEnabled returns true when debug diagnostics are enabled.
func DebugEnabled() bool {
	return DebugFlag
}

// SetDebugWriter overrides the writer used for debug diagnostics.
// Tests use this to capture stderr-only output.
func SetDebugWriter(w io.Writer) {
	if w == nil {
		debugWriter = io.Discard
		return
	}

	debugWriter = w
}

// SetJSONErrorWriter overrides the writer used for JSON errors when debug mode
// is enabled. Tests use this to keep machine-readable JSON isolated from debug
// diagnostics written to stderr.
func SetJSONErrorWriter(w io.Writer) {
	if w == nil {
		jsonErrorWriter = io.Discard
		return
	}

	jsonErrorWriter = w
}

// JSONErrorWriter returns the writer that should receive machine-readable JSON
// errors. Under debug mode, it is isolated from stderr diagnostics.
func JSONErrorWriter(fallback io.Writer) io.Writer {
	if DebugEnabled() && IsJSON() {
		return jsonErrorWriter
	}

	return fallback
}

// SanitizeDebugString redacts sensitive values before they are written to
// debug logs.
func SanitizeDebugString(value string) string {
	sanitized := strings.Join(strings.Fields(value), " ")
	for _, redactor := range debugStringRedactors {
		sanitized = redactor.pattern.ReplaceAllString(sanitized, redactor.replace)
	}

	return sanitized
}

// SanitizeDebugStrings applies SanitizeDebugString to each value.
func SanitizeDebugStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	sanitized := make([]string, len(values))
	for i, value := range values {
		sanitized[i] = SanitizeDebugString(value)
	}

	return sanitized
}

// SanitizeDebugMap applies SanitizeDebugString to each map value.
func SanitizeDebugMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	sanitized := make(map[string]string, len(values))
	for key, value := range values {
		sanitized[key] = SanitizeDebugString(value)
	}

	return sanitized
}

// Debugf writes a single debug line when debug mode is enabled.
func Debugf(format string, args ...any) {
	if !DebugEnabled() || debugWriter == nil {
		return
	}

	_, _ = fmt.Fprintf(debugWriter, "[debug] "+format+"\n", args...)
}
