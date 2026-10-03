package output

import (
	"fmt"
	"regexp"
	"strings"
)

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

// Debugf writes a single debug line to DebugOut when debug mode is enabled.
func (o *Options) Debugf(format string, args ...any) {
	if !o.Debug || o.DebugOut == nil {
		return
	}

	_, _ = fmt.Fprintf(o.DebugOut, "[debug] "+format+"\n", args...)
}
