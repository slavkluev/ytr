package cmd_test

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd"
	"github.com/slavkluev/ytr/internal/cmd/queue"
	"github.com/slavkluev/ytr/internal/cmd/runner"
)

func TestCommandTree(t *testing.T) {
	t.Parallel()

	root := cmd.RootCmd()

	if root.Use != "ytr" {
		t.Errorf("root command Use = %q, want %q", root.Use, "ytr")
	}

	if !root.HasSubCommands() {
		t.Fatal("root command has no subcommands, expected at least 'version'")
	}

	// Find version command in subcommands
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("root command does not have 'version' subcommand")
	}
}

func TestDebugFlagRegistered(t *testing.T) {
	t.Parallel()

	root := cmd.RootCmd()

	flag := root.PersistentFlags().Lookup("debug")
	if flag == nil {
		t.Fatal("expected persistent --debug flag to be registered")
	}
	if !flag.Hidden {
		t.Error("--debug is not hidden, so root help lists it")
	}
}

func TestWorklogAndChecklistRegistered(t *testing.T) {
	t.Parallel()

	root := cmd.RootCmd()
	subNames := make(map[string]bool)
	for _, sub := range root.Commands() {
		subNames[sub.Name()] = true
	}
	for _, name := range []string{"worklog", "checklist"} {
		if !subNames[name] {
			t.Errorf("%q not registered on root command", name)
		}
	}
}

func TestQueueContextRegistered(t *testing.T) {
	t.Parallel()

	root := cmd.RootCmd()

	var queueCmd *cobra.Command
	for _, sub := range root.Commands() {
		if sub.Name() == "queue" {
			queueCmd = sub
			break
		}
	}

	if queueCmd == nil {
		t.Fatal("'queue' not registered on root command")
	}

	var contextCmd *cobra.Command
	for _, sub := range queueCmd.Commands() {
		if sub.Name() == "context" {
			contextCmd = sub
			break
		}
	}

	if contextCmd == nil {
		t.Fatal("queue subcommand \"context\" not registered")
	}

	fields, ok := runner.Fields(contextCmd)
	if !ok || !slices.Equal(fields, queue.QueueContextFields) {
		t.Errorf("runner.Fields(%q) = %q (set %v), want %q",
			contextCmd.CommandPath(), fields, ok, queue.QueueContextFields)
	}
}
