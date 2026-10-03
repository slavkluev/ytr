package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/slavkluev/ytr/internal/output"
)

// TestJSONCompletionOffersEveryLeafItsFields completes --json on every leaf of
// the tree. A leaf missing from the table must offer nothing, so a leaf that
// gains fields, or loses the ones it had, fails here.
func TestJSONCompletionOffersEveryLeafItsFields(t *testing.T) {
	refdata := []string{"id", "key", "name"}
	issueDetail := []string{
		"key", "summary", "status", "priority", "type", "author", "authorId",
		"assignee", "assigneeId", "createdAt", "updatedAt", "description",
	}
	comment := []string{"id", "author", "authorId", "body", "createdAt", "updatedAt"}
	link := []string{"id", "type", "issue", "summary"}
	worklog := []string{"id", "author", "authorId", "duration", "start", "comment"}
	checklist := []string{"id", "text", "checked", "assignee", "assigneeId"}
	bulk := []string{
		"id", "status", "statusText", "totalIssues", "totalCompletedIssues",
		"executionIssuePercent", "executionChunkPercent", "createdBy", "createdById", "createdAt",
	}
	component := []string{"id", "name", "queue", "lead", "leadId", "description", "assignAuto"}
	deleted := []string{"id", "deleted"}
	userDetail := []string{
		"uid", "display", "login", "email", "firstName", "lastName", "dismissed", "hasLicense", "external",
	}

	want := map[string][]string{
		"issue changelog": {
			"date", "author", "authorId", "type", "transport", "fields", "comments",
			"links", "attachments", "worklog", "relatedResolutions",
		},
		"issue create": issueDetail,
		"issue list": {
			"key", "summary", "status", "priority", "type", "assignee", "assigneeId", "createdAt", "updatedAt",
		},
		"issue transition": {"key", "transition"},
		"issue update":     issueDetail,
		"issue view":       issueDetail,
		"comment create":   comment,
		"comment delete":   deleted,
		"comment edit":     comment,
		"comment list":     comment,
		"link create":      link,
		"link delete":      deleted,
		"link list":        link,
		"worklog create":   worklog,
		"worklog delete":   deleted,
		"worklog edit":     worklog,
		"worklog list":     worklog,
		"checklist create": checklist,
		"checklist delete": deleted,
		"checklist edit":   checklist,
		"checklist list":   checklist,
		"bulk move":        bulk,
		"bulk status":      bulk,
		"bulk transition":  bulk,
		"bulk update":      bulk,
		"status list":      refdata,
		"priority list":    refdata,
		"resolution list":  refdata,
		"issuetype list":   refdata,
		"field get": {
			"id", "key", "name", "type", "schema", "items", "required", "readonly",
			"category", "queue", "options", "queueOptions", "defaultOptions", "description",
		},
		"field list": {
			"id", "key", "name", "schema", "items", "readonly", "options", "queueOptions", "defaultOptions",
		},
		"queue context": {
			"key", "name", "defaultType", "defaultPriority", "issueTypes", "statuses", "workflows",
			"components", "requiredFields", "localFields", "globalFields", "incomplete",
		},
		"queue list": {"key", "name", "lead", "leadId"},
		"queue view": {
			"key", "name", "description", "lead", "leadId", "defaultType", "defaultPriority",
			"assignAuto", "allowExternals",
		},
		"component create": component,
		"component delete": deleted,
		"component edit":   component,
		"component get":    component,
		"component list":   component,
		"user get":         userDetail,
		"user list":        {"uid", "display", "login", "email"},
		"user myself":      userDetail,
		"version":          {"version", "commit", "date", "goVersion", "os", "arch"},
	}

	completed := 0
	walkCommands(newRootCmd(&output.Options{}), func(leaf *cobra.Command) {
		if leaf.HasSubCommands() {
			return
		}
		path := argPath(leaf)
		name := strings.Join(path, " ")

		t.Run(name, func(t *testing.T) {
			res := runCLI(t, nil, slices.Concat([]string{"__complete"}, path, []string{"--json", ""})...)

			// Cobra ends the offers with a ":<directive>" line.
			offered, _, _ := strings.Cut("\n"+res.Stdout, "\n:")
			var got []string
			if offered != "" {
				got = strings.Split(strings.TrimPrefix(offered, "\n"), "\n")
			}
			if !slices.Equal(got, want[name]) {
				t.Errorf("completion offers %q, want %q (stdout: %q)", got, want[name], res.Stdout)
			}
		})
		if _, ok := want[name]; ok {
			completed++
		}
	})

	if completed != len(want) {
		t.Errorf("the walk reached %d of the %d leaves the table lists", completed, len(want))
	}
}
