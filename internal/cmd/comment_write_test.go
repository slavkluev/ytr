package cmd

import (
	"net/http"
	"slices"
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

const (
	commentAnswer = `{"id": 555, "longId": "5fa15a24ac894475000000aa", "text": "Fixed in abc123",
		"createdBy": {"id": "uid-a", "display": "Иван Петров"},
		"createdAt": "2026-09-17T09:05:00.000+0300", "updatedAt": "2026-09-18T10:00:00.000+0000"}`
	commentItemJSON = `{"id": "555", "author": "Иван Петров", "authorId": "uid-a", "body": "Fixed in abc123",
		"createdAt": "2026-09-17T09:05:00+03:00", "updatedAt": "2026-09-18T10:00:00Z"}`
)

func TestCommentCreate(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/comments"
	created := trackerPOST(path, commentAnswer)
	create := func(extra ...string) []string {
		return slices.Concat([]string{"comment", "create", "PROJ-1"}, extra)
	}

	runLeafRows(t, []leafRow{
		{
			name: "Flag body", args: create("--body", "Fixed in abc123"), exchanges: []faketracker.Exchange{created},
			body: `{"text": "Fixed in abc123"}`, json: commentItemJSON,
		},
		{
			name: "JSON", args: create("--body", "Fixed in abc123", "--json", "id,body"),
			exchanges: []faketracker.Exchange{created}, json: `{"id": "555", "body": "Fixed in abc123"}`,
		},
		{
			name: "A bare comment", args: create("--body", "x"),
			exchanges: []faketracker.Exchange{trackerPOST(path, `{"id": 555}`)},
			json:      `{"id": "555", "author": "", "authorId": "", "body": "", "createdAt": ""}`,
		},
		{
			name: "jq", args: create("--body", "x", "--jq", ".authorId"), exchanges: []faketracker.Exchange{created},
			stdout: "uid-a\n",
		},
		{
			name: "Bad value", args: create("--body", "hello\x00world"), code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"control character U+0000 at position 5 in body"`},
		},
		{
			name:   "Bad arg",
			args:   []string{"comment", "create", "bad-key", "--body", "x"},
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid issue key \"bad-key\": expected format QUEUE-123"`},
		},
		{
			name: "Extra arg", args: []string{"comment", "create", "PROJ-1", "PROJ-2", "--body", "x"},
			code: ytrerrors.ExitUserError, stderr: []string{"accepts 1 arg(s), received 2"},
		},
		failureRow(trackerNotFoundOn(http.MethodPost, path, "Issue not found"),
			create("--body", "x")...),
	})
}

func TestCommentEdit(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/comments/555"
	edited := trackerPATCH(path, commentAnswer)
	edit := func(extra ...string) []string {
		return slices.Concat([]string{"comment", "edit", "PROJ-1", "555"}, extra)
	}

	runLeafRows(t, []leafRow{
		{
			name: "Flag body", args: edit("--body", "Fixed in abc123"), exchanges: []faketracker.Exchange{edited},
			body: `{"text": "Fixed in abc123"}`, json: commentItemJSON,
		},
		{
			name: "JSON body", args: edit("--from-json", `{"text": "Fixed in abc123"}`),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"text": "Fixed in abc123"}`, json: commentItemJSON,
		},
		{
			name: "JSON body on stdin", args: edit("--from-json", "-"), stdin: `{"text": "Fixed in abc123"}`,
			exchanges: []faketracker.Exchange{edited},
			body:      `{"text": "Fixed in abc123"}`, json: commentItemJSON,
		},
		{
			name:      "JSON body with a key no flag sets",
			args:      edit("--from-json", `{"text": "x", "summonees": ["uid-b"]}`),
			exchanges: []faketracker.Exchange{edited},
			body:      `{"text": "x", "summonees": ["uid-b"]}`,
			json:      commentItemJSON,
		},
		{
			name: "The ID Tracker sent", args: []string{"comment", "edit", "PROJ-1", "0555", "--body", "x"},
			exchanges: []faketracker.Exchange{
				trackerPATCH("/v3/issues/PROJ-1/comments/0555", commentAnswer),
			},
			json: commentItemJSON,
		},
		{
			name: "JSON", args: edit("--body", "x", "--json", "id,authorId"),
			exchanges: []faketracker.Exchange{edited}, json: `{"id": "555", "authorId": "uid-a"}`,
		},
		{
			name: "Bad value", args: edit("--body", "hello\x00world"), code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"control character U+0000 at position 5 in body"`},
		},
		{
			name: "Bad value signed out", args: edit("--body", "a\x00"), signedOut: true, code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"control character U+0000 at position 1 in body"`},
		},
		{
			name: "Bad value in JSON", args: edit("--from-json", `{"text": "a\u0000"}`), code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"control character U+0000 at position 1 in body"`},
		},
		{
			name: "Unknown key", args: edit("--from-json", `{"text": "hi", "bogus": 1}`),
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"code":"invalid_field"`, `"invalidFields":["bogus"]`},
		},
		{
			name: "Malformed JSON", args: edit("--from-json", `{"text":`), code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid JSON input: unexpected end of JSON input"`},
		},
		{
			name: "Unreadable JSON file", args: edit("--from-json", "@"+t.TempDir()+"/missing.json"),
			code: ytrerrors.ExitUserError, stderr: []string{`"message":"failed to read JSON input: open `},
		},
		{
			name:   "Bad arg",
			args:   []string{"comment", "edit", "PROJ-1", "abc", "--body", "x"},
			code:   ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid comment ID \"abc\": expected a positive integer"`},
		},
		{
			name: "Bad issue key", args: []string{"comment", "edit", "bad", "555", "--body", "x"},
			code: ytrerrors.ExitUserError, stderr: []string{`invalid issue key \"bad\"`},
		},
		failureRow(trackerNotFoundOn(http.MethodPatch, path, "Comment not found"),
			edit("--body", "x")...),
	})
}

func TestCommentDelete(t *testing.T) {
	t.Parallel()

	const path = "/v3/issues/PROJ-1/comments/555"
	args := []string{"comment", "delete", "PROJ-1", "555"}

	runLeafRows(t, slices.Concat(deleteRows(args, trackerDELETE(path), "555"), []leafRow{
		{
			name: "ID as given", args: []string{"comment", "delete", "PROJ-1", "0555"},
			exchanges: []faketracker.Exchange{trackerDELETE("/v3/issues/PROJ-1/comments/0555")},
			json:      `{"id": "0555", "deleted": true}`,
		},
		{
			name: "Bad arg", args: []string{"comment", "delete", "PROJ-1", "abc"}, code: ytrerrors.ExitUserError,
			stderr: []string{`"message":"invalid comment ID \"abc\": expected a positive integer"`},
		},
		{
			name: "Bad issue key", args: []string{"comment", "delete", "bad", "555"}, code: ytrerrors.ExitUserError,
			stderr: []string{`invalid issue key \"bad\"`},
		},
		failureRow(trackerNotFoundOn(http.MethodDelete, path, "Comment not found"),
			args...),
		helpRow("comment delete", "Delete a comment from a Yandex Tracker issue.\n\nJSON FIELDS\n  id, deleted\n"),
	}))
}
