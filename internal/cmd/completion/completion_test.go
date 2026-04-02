package completion_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/cmd"
)

func TestBashCompletion(t *testing.T) {
	root := cmd.RootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "bash"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("completion bash failed: %v", err)
	}

	output := buf.String()
	if len(output) < 100 {
		t.Error("bash completion output is too short")
	}
	if !strings.Contains(output, "ytr") {
		t.Error("bash completion output does not reference ytr")
	}
}

func TestZshCompletion(t *testing.T) {
	root := cmd.RootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "zsh"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("completion zsh failed: %v", err)
	}

	output := buf.String()
	if len(output) < 100 {
		t.Error("zsh completion output is too short")
	}
}

func TestFishCompletion(t *testing.T) {
	root := cmd.RootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"completion", "fish"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("completion fish failed: %v", err)
	}

	output := buf.String()
	if len(output) < 100 {
		t.Error("fish completion output is too short")
	}
	if !strings.Contains(output, "complete") {
		t.Error("fish completion output missing 'complete' command")
	}
}

func TestCompletionInCommandTree(t *testing.T) {
	root := cmd.RootCmd()
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "completion" {
			found = true
			break
		}
	}
	if !found {
		t.Error("root command does not have 'completion' subcommand")
	}
}
