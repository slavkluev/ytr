package cmd

import (
	"encoding/json"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

// badOptionsField is a field whose optionsProvider values are neither a list
// nor a per-queue object, which the library refuses to decode.
const badOptionsField = `{"key": "size", "optionsProvider": {"values": "S"}}`

func optionDecodeError(t *testing.T) string {
	t.Helper()

	var field tracker.Field
	err := json.Unmarshal([]byte(badOptionsField), &field)
	if err == nil {
		t.Fatal("the library decoded a string optionsProvider.values")
	}

	return err.Error()
}

func TestFieldList(t *testing.T) {
	t.Parallel()

	const path = "/v3/fields"
	fields := trackerGET(path, `[
		{"id": "summary", "key": "summary", "name": "Summary",
		 "schema": {"type": "string", "required": true}, "readonly": true},
		{"id": "tags", "key": "tags", "name": "Tags", "schema": {"type": "array", "items": "string"}, "readonly": false},
		{"id": "possibleSpam", "key": "possibleSpam", "name": "Possible spam", "schema": {"type": "integer"},
		 "readonly": false, "optionsProvider": {"type": "FixedListOptionsProvider", "values": [0, 1]}},
		{"id": "stand", "key": "stand", "name": "Board", "schema": {"type": "string"},
		 "optionsProvider": {"type": "QueueFixedListOptionsProvider",
		  "values": {"DIRECT": ["Test", "Beta"], "ALPHA": ["A1"]}, "defaults": ["Test"]}},
		{"id": "team", "key": "team", "name": "Team", "optionsProvider": {"type": "TeamOptionsProvider"}},
		{}
	]`)
	local := trackerGET("/v3/queues/PROJ/localFields", `[
		{"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "name": "Size", "schema": {"type": "string"},
		 "readonly": false, "optionsProvider": {"values": ["S", "M", "L"]}}
	]`)
	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"field", "list"}, exchanges: []faketracker.Exchange{fields},
			json: `[
				{"id": "summary", "key": "summary", "name": "Summary", "schema": "string", "readonly": true},
				{"id": "tags", "key": "tags", "name": "Tags", "schema": "array", "items": "string", "readonly": false},
				{"id": "possibleSpam", "key": "possibleSpam", "name": "Possible spam", "schema": "integer",
				 "readonly": false, "options": [0, 1]},
				{"id": "stand", "key": "stand", "name": "Board", "schema": "string", "readonly": false,
				 "queueOptions": {"ALPHA": ["A1"], "DIRECT": ["Test", "Beta"]}, "defaultOptions": ["Test"]},
				{"id": "team", "key": "team", "name": "Team", "readonly": false},
				{"id": "", "key": "", "name": "", "readonly": false}
			]`,
		},
		{
			name: "Local fields", args: []string{"field", "list", "--queue", "PROJ"},
			exchanges: []faketracker.Exchange{local},
			json: `[{"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "name": "Size", "schema": "string",
				"readonly": false, "options": ["S", "M", "L"]}]`,
		},
		{
			name: "Local fields JSON", args: []string{"field", "list", "--queue", "PROJ", "--json", "id,key,options"},
			exchanges: []faketracker.Exchange{local},
			json:      `[{"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "options": ["S", "M", "L"]}]`,
		},
		{
			name: "jq", args: []string{"field", "list", "--jq", ".[2].options"},
			exchanges: []faketracker.Exchange{fields}, stdout: "[0,1]\n",
		},
		{
			name: "Empty", args: []string{"field", "list"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[]`)}, json: `[]`,
		},
		{
			name: "Options Tracker cannot send", args: []string{"field", "list"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[`+badOptionsField+`]`)},
			code:      ytrerrors.ExitUserError, stderr: []string{optionDecodeError(t)},
		},
		named(
			"Empty selection for local fields",
			emptySelectionRow("field list", []string{"--queue", "PROJ"}, "id", "key", "name", "schema", "items",
				"readonly", "options", "queueOptions", "defaultOptions"),
		),
		notFoundRow("/v3/queues/NOPE/localFields", "field", "list", "--queue", "NOPE"),
	})
}

func TestFieldGet(t *testing.T) {
	t.Parallel()

	const path = "/v3/fields/summary"
	summary := trackerGET(path, `{"id": "summary", "key": "summary", "name": "Summary", "type": "standard",
		"schema": {"type": "string", "required": true}, "readonly": false,
		"category": {"display": "System"}, "description": "Issue summary"}`)
	local := trackerGET("/v3/queues/PROJ/localFields/size", `{"id": "5d0e4f1a2b3c4d5e6f708192--size",
		"key": "size", "name": "Size", "type": "local", "schema": {"type": "array", "items": "string"},
		"readonly": false, "queue": {"key": "PROJ"},
		"optionsProvider": {"values": {"ZETA": ["Z1"], "ALPHA": ["A1", "A2"]}, "defaults": ["A1"]}}`)
	numeric := trackerGET("/v3/fields/possibleSpam", `{"id": "possibleSpam", "key": "possibleSpam",
		"name": "Possible spam", "type": "standard", "schema": {"type": "integer", "required": false},
		"readonly": false, "optionsProvider": {"type": "FixedListOptionsProvider", "values": [0, 1]}}`)
	textOptions := trackerGET("/v3/fields/issueType",
		`{"key": "issueType", "optionsProvider": {"values": ["bug", "task"]}}`)
	team := trackerGET("/v3/fields/team", `{"id": "team", "key": "team", "name": "Team",
		"optionsProvider": {"type": "TeamOptionsProvider"}}`)
	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"field", "get", "summary"}, exchanges: []faketracker.Exchange{summary},
			json: `{"id": "summary", "key": "summary", "name": "Summary", "type": "standard", "schema": "string",
				"required": true, "readonly": false, "category": "System", "description": "Issue summary"}`,
		},
		{
			name: "Local field", args: []string{"field", "get", "size", "--queue", "PROJ"},
			exchanges: []faketracker.Exchange{local},
			json: `{"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "name": "Size", "type": "local",
				"schema": "array", "items": "string", "required": false, "readonly": false, "queue": "PROJ",
				"queueOptions": {"ALPHA": ["A1", "A2"], "ZETA": ["Z1"]}, "defaultOptions": ["A1"]}`,
		},
		{
			name: "Numeric options JSON", args: []string{"field", "get", "possibleSpam", "--json", "key,options"},
			exchanges: []faketracker.Exchange{numeric}, json: `{"key": "possibleSpam", "options": [0, 1]}`,
		},
		{
			name: "Text options JSON", args: []string{"field", "get", "issueType", "--json", "key,options"},
			exchanges: []faketracker.Exchange{textOptions}, json: `{"key": "issueType", "options": ["bug", "task"]}`,
		},
		{
			name: "A field with only a key and options", args: []string{"field", "get", "issueType"},
			exchanges: []faketracker.Exchange{textOptions},
			json: `{"id": "", "key": "issueType", "name": "", "required": false, "readonly": false,
				"options": ["bug", "task"]}`,
		},
		{
			name:      "No options JSON",
			args:      []string{"field", "get", "team", "--json", "key,options,queueOptions,defaultOptions"},
			exchanges: []faketracker.Exchange{team},
			json:      `{"key": "team"}`,
		},
		{
			name: "jq", args: []string{"field", "get", "summary", "--jq", ".schema"},
			exchanges: []faketracker.Exchange{summary}, stdout: "string\n",
		},
		{
			name: "Blank key", args: []string{"field", "get", " "},
			code: ytrerrors.ExitUserError, stderr: []string{"invalid field key: expected a non-empty value"},
		},
		{
			name: "Options Tracker cannot send", args: []string{"field", "get", "size"},
			exchanges: []faketracker.Exchange{trackerGET("/v3/fields/size", badOptionsField)},
			code:      ytrerrors.ExitUserError, stderr: []string{optionDecodeError(t)},
		},
		notFoundRow("/v3/queues/PROJ/localFields/nope", "field", "get", "nope", "--queue", "PROJ"),
	})
}
