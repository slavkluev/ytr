package version_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/cmd/version"
	"github.com/slavkluev/ytr/internal/output"
)

func TestVersionHuman(t *testing.T) {
	// Ensure JSON mode is off
	output.JSONFields = nil
	defer output.ResetFlags()

	cmd := version.NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()

	checks := []string{
		"ytr version",
		"commit:",
		"go:",
		"os/arch:",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestVersionJSON(t *testing.T) {
	output.JSONFields = version.VersionFields
	defer output.ResetFlags()

	cmd := version.NewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.Bytes()

	if !json.Valid(out) {
		t.Fatalf("output is not valid JSON: %s", out)
	}

	var result map[string]string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	requiredKeys := []string{"version", "commit", "goVersion", "os", "arch"}
	for _, key := range requiredKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("JSON output missing key %q, got: %v", key, result)
		}
	}

	// Verify no ANSI escape sequences in JSON output
	if strings.Contains(string(out), "\x1b") {
		t.Errorf("JSON output contains ANSI escape sequences: %s", out)
	}
}

// TestVersionJSONEmptyShowsFieldHint covers Info #4: `version --json=` (an
// explicit empty value) must show the available-fields hint, not fall through
// to table output with exit 0. Detection relies on cmd.Flags().Changed("json"),
// so the test wires a --json flag mirroring the root persistent flag.
func TestVersionJSONEmptyShowsFieldHint(t *testing.T) {
	output.ResetFlags()
	defer output.ResetFlags()

	cmd := version.NewCmd()
	cmd.PersistentFlags().StringSliceVar(&output.JSONFields, "json", nil, "")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--json="})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a field-hint error for `version --json=`, got nil")
	}
	if !strings.Contains(buf.String(), "Available fields for version") {
		t.Errorf("expected available-fields hint, got:\n%s", buf.String())
	}
}

func TestVersionNoArgs(t *testing.T) {
	cmd := version.NewCmd()
	cmd.SetArgs([]string{"extra-arg"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for extra arguments, got nil")
	}
}
