// Package jsonfields provides a registry for JSON field completions.
// Command packages register their available --json field names here, and a
// single root-level completion function delegates lookups. Commands built by
// internal/cmd/runner carry their fields on the command instead and are not
// registered here.
package jsonfields

import "sync"

var (
	mu       sync.Mutex
	registry = make(map[string][]string)
)

// Register stores JSON field names for a command path.
// Called by command packages in their NewCmd() or subcommand constructors;
// commands built by internal/cmd/runner carry their fields on the command
// instead. commandPath should match cmd.CommandPath() output, e.g. "ytr issue list".
func Register(commandPath string, fields []string) {
	mu.Lock()
	defer mu.Unlock()
	registry[commandPath] = fields
}

// Get returns JSON fields for a command path.
func Get(commandPath string) ([]string, bool) {
	mu.Lock()
	defer mu.Unlock()
	f, ok := registry[commandPath]
	return f, ok
}
