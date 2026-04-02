package cmd_test

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/cmd"
	"github.com/slavkluev/ytr/internal/output"
)

func TestCommandTree(t *testing.T) {
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

func TestMutuallyExclusiveFlags(t *testing.T) {
	root := cmd.RootCmd()
	root.SetArgs([]string{"version", "--json", "key", "--quiet"})
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)

	err := root.Execute()
	if err == nil {
		t.Error("expected error when both --json and --quiet are set, got nil")
	}

	// Reset flag state
	output.ResetFlags()
}

func TestMutuallyExclusiveJQAndQuiet(t *testing.T) {
	root := cmd.RootCmd()
	root.SetArgs([]string{"version", "--jq", ".version", "--quiet"})
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)

	err := root.Execute()
	if err == nil {
		t.Error("expected error when both --jq and --quiet are set, got nil")
	}

	output.ResetFlags()
}

func TestDebugFlagRegistered(t *testing.T) {
	root := cmd.RootCmd()

	if flag := root.PersistentFlags().Lookup("debug"); flag == nil {
		t.Fatal("expected persistent --debug flag to be registered")
	}
}

func TestCommandGroups(t *testing.T) {
	root := cmd.RootCmd()
	groups := root.Groups()
	if len(groups) < 5 {
		t.Fatalf("expected at least 5 command groups, got %d", len(groups))
	}

	expectedGroups := []string{"issue-tracking", "reference-data", "organization", "account", "system"}
	groupIDs := make(map[string]bool)
	for _, g := range groups {
		groupIDs[g.ID] = true
	}
	for _, id := range expectedGroups {
		if !groupIDs[id] {
			t.Errorf("missing command group %q", id)
		}
	}
}

func TestWorklogAndChecklistRegistered(t *testing.T) {
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

func TestBulkRegistered(t *testing.T) {
	root := cmd.RootCmd()

	var bulkCmd *cobra.Command
	for _, sub := range root.Commands() {
		if sub.Name() == "bulk" {
			bulkCmd = sub
			break
		}
	}

	if bulkCmd == nil {
		t.Fatal("'bulk' not registered on root command")
	}

	if bulkCmd.GroupID != "issue-tracking" {
		t.Errorf("bulk command GroupID = %q, want %q", bulkCmd.GroupID, "issue-tracking")
	}

	// Verify bulk subcommands.
	subNames := make(map[string]bool)
	for _, sub := range bulkCmd.Commands() {
		subNames[sub.Name()] = true
	}

	for _, name := range []string{"status", "move", "update", "transition"} {
		if !subNames[name] {
			t.Errorf("bulk subcommand %q not registered", name)
		}
	}
}

func TestAllCommandsGrouped(t *testing.T) {
	root := cmd.RootCmd()
	for _, c := range root.Commands() {
		if c.Name() == "help" {
			continue // Cobra's help command is auto-grouped via SetHelpCommandGroupID
		}
		if c.GroupID == "" {
			t.Errorf("command %q has no GroupID, will appear under 'Additional Commands'", c.Name())
		}
	}
}
