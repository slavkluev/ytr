// Package jsonfields provides a registry for JSON field completions.
// Each command package registers its available --json field names here,
// and a single root-level completion function delegates lookups.
package jsonfields

import "sync"

var (
	mu       sync.Mutex
	registry = make(map[string][]string)
)

// Register stores JSON field names for a command path.
// Called by each command package in its NewCmd() or subcommand constructor.
// commandPath should match cmd.CommandPath() output, e.g. "ytr issue list".
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
