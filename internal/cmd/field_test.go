package cmd

import (
	"encoding/json"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
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
	all := "id,key,name,schema,items,readonly,options,queueOptions,defaultOptions"
	//nolint:dupword // A global field's id is its key.
	globalFieldsTable := "ID\tKEY\tNAME\tSCHEMA\tREADONLY\n" +
		"summary\tsummary\tSummary\tstring\tyes\n" +
		"tags\ttags\tTags\tarray\tno\n" +
		"possibleSpam\tpossibleSpam\tPossible spam\tinteger\tno\n" +
		"stand\tstand\tBoard\tstring\tno\n" +
		"team\tteam\tTeam\t-\tno\n" +
		"-\t-\t-\t-\tno\n"

	runLeafRows(t, []leafRow{
		{
			name: "JSON", args: []string{"field", "list", "--json", all}, exchanges: []faketracker.Exchange{fields},
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
			name: "Table", args: []string{"field", "list"}, exchanges: []faketracker.Exchange{fields},
			stdout: globalFieldsTable,
		},
		{
			name: "Local fields", args: []string{"field", "list", "--queue", "PROJ"},
			exchanges: []faketracker.Exchange{local},
			stdout: "ID\tKEY\tNAME\tSCHEMA\tREADONLY\n" +
				"5d0e4f1a2b3c4d5e6f708192--size\tsize\tSize\tstring\tno\n",
		},
		{
			name: "Local fields JSON", args: []string{"field", "list", "--queue", "PROJ", "--json", "id,key,options"},
			exchanges: []faketracker.Exchange{local},
			json:      `[{"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "options": ["S", "M", "L"]}]`,
		},
		{
			name: "TTY", args: []string{"field", "list"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{fields},
			holds:     []string{"ID", "KEY", "NAME", "SCHEMA", "READONLY", "summary", "Summary", "string", "yes"},
			check:     assertAlignedTable,
		},
		{
			name: "Quiet", args: []string{"field", "list", "--quiet"},
			exchanges: []faketracker.Exchange{fields}, stdout: "summary\ntags\npossibleSpam\nstand\nteam\n\n",
		},
		{
			name: "jq", args: []string{"field", "list", "--jq", ".[2].options"},
			exchanges: []faketracker.Exchange{fields}, stdout: "[0,1]\n",
		},
		{
			name: "Empty", args: []string{"field", "list"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[]`)}, stdout: "No fields found\n",
		},
		{
			name: "Options Tracker cannot send", args: []string{"field", "list"},
			exchanges: []faketracker.Exchange{trackerGET(path, `[`+badOptionsField+`]`)},
			code:      ytrerrors.ExitUserError, stderr: []string{optionDecodeError(t)},
		},
		fieldHintRow("field list", nil, "id", "key", "name", "schema", "items", "readonly",
			"options", "queueOptions", "defaultOptions"),
		named(
			"Field hint for local fields",
			fieldHintRow("field list", []string{"--queue", "PROJ"}, "id", "key", "name", "schema", "items",
				"readonly", "options", "queueOptions", "defaultOptions"),
		),
		notFoundRow("/v3/queues/NOPE/localFields", "field", "list", "--queue", "NOPE", "--json", "id"),
		helpRow("field list", "JSON FIELDS\n"+
			"  id, key, name, schema, items, readonly, options, queueOptions, defaultOptions\n\n"+
			"SEE ALSO\n  ytr field get  - Show field details\n"),
	})
}

func TestFieldGet(t *testing.T) {
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
	all := "id,key,name,type,schema,items,required,readonly,category,queue,options,queueOptions,defaultOptions,description"

	runLeafRows(t, []leafRow{
		{
			name: "JSON", args: []string{"field", "get", "summary", "--json", all},
			exchanges: []faketracker.Exchange{summary},
			json: `{"id": "summary", "key": "summary", "name": "Summary", "type": "standard", "schema": "string",
				"required": true, "readonly": false, "category": "System", "description": "Issue summary"}`,
		},
		{
			name: "Card", args: []string{"field", "get", "summary"}, exchanges: []faketracker.Exchange{summary},
			stdout: "ID\tsummary\nKey\tsummary\nName\tSummary\nType\tstandard\nSchema\tstring (required)\n" +
				"Readonly\tno\nCategory\tSystem\n\nDescription:\n  Issue summary\n",
		},
		{
			name: "Local field JSON", args: []string{"field", "get", "size", "--queue", "PROJ", "--json", all},
			exchanges: []faketracker.Exchange{local},
			json: `{"id": "5d0e4f1a2b3c4d5e6f708192--size", "key": "size", "name": "Size", "type": "local",
				"schema": "array", "items": "string", "required": false, "readonly": false, "queue": "PROJ",
				"queueOptions": {"ALPHA": ["A1", "A2"], "ZETA": ["Z1"]}, "defaultOptions": ["A1"]}`,
		},
		{
			name: "Local field card", args: []string{"field", "get", "size", "--queue", "PROJ"},
			exchanges: []faketracker.Exchange{local},
			stdout: "ID\t5d0e4f1a2b3c4d5e6f708192--size\nKey\tsize\nName\tSize\nType\tlocal\n" +
				"Schema\tarray of string\nReadonly\tno\nQueue\tPROJ\n" +
				"Options (ALPHA)\tA1, A2\nOptions (ZETA)\tZ1\nDefault options\tA1\n",
		},
		{
			name: "Numeric options JSON", args: []string{"field", "get", "possibleSpam", "--json", "key,options"},
			exchanges: []faketracker.Exchange{numeric}, json: `{"key": "possibleSpam", "options": [0, 1]}`,
		},
		{
			name: "Numeric options card", args: []string{"field", "get", "possibleSpam"},
			exchanges: []faketracker.Exchange{numeric},
			stdout: "ID\tpossibleSpam\nKey\tpossibleSpam\nName\tPossible spam\nType\tstandard\n" +
				"Schema\tinteger\nReadonly\tno\nOptions\t0, 1\n",
		},
		{
			name: "Text options JSON", args: []string{"field", "get", "issueType", "--json", "key,options"},
			exchanges: []faketracker.Exchange{textOptions}, json: `{"key": "issueType", "options": ["bug", "task"]}`,
		},
		{
			name: "Text options card", args: []string{"field", "get", "issueType"},
			exchanges: []faketracker.Exchange{textOptions},
			stdout:    "ID\t-\nKey\tissueType\nName\t-\nType\t-\nSchema\t-\nReadonly\tno\nOptions\tbug, task\n",
		},
		{
			name:      "No options JSON",
			args:      []string{"field", "get", "team", "--json", "key,options,queueOptions,defaultOptions"},
			exchanges: []faketracker.Exchange{team},
			json:      `{"key": "team"}`,
		},
		{
			name: "No options card", args: []string{"field", "get", "team"}, exchanges: []faketracker.Exchange{team},
			stdout: "ID\tteam\nKey\tteam\nName\tTeam\nType\t-\nSchema\t-\nReadonly\tno\n",
		},
		{
			name: "TTY", args: []string{"field", "get", "summary"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{summary},
			holds: []string{"ID:  summary\nKey:  summary\nName:  Summary\nType:  standard\n" +
				"Schema:  string (required)\nReadonly:  no\nCategory:  System\n\nDescription:\n  Issue summary\n"},
		},
		{
			name: "Quiet", args: []string{"field", "get", "summary", "--quiet"},
			exchanges: []faketracker.Exchange{summary}, stdout: "summary\n",
		},
		{
			name: "jq", args: []string{"field", "get", "summary", "--jq", ".schema"},
			exchanges: []faketracker.Exchange{summary}, stdout: "string\n",
		},
		{
			name: "Blank key", args: []string{"field", "get", " ", "--quiet"},
			code: ytrerrors.ExitUserError, stderr: []string{"invalid field key: expected a non-empty value"},
		},
		{
			name: "Options Tracker cannot send", args: []string{"field", "get", "size"},
			exchanges: []faketracker.Exchange{trackerGET("/v3/fields/size", badOptionsField)},
			code:      ytrerrors.ExitUserError, stderr: []string{optionDecodeError(t)},
		},
		fieldHintRow("field get", []string{"summary"}, "id", "key", "name", "type", "schema", "items",
			"required", "readonly", "category", "queue", "options", "queueOptions", "defaultOptions", "description"),
		notFoundRow("/v3/queues/PROJ/localFields/nope", "field", "get", "nope", "--queue", "PROJ", "--json", "id"),
		helpRow("field get", "JSON FIELDS\n  id, key, name, type, schema, items, required, readonly, category, "+
			"queue, options, queueOptions, defaultOptions, description\n\n"+
			"SEE ALSO\n  ytr field list  - List available fields\n"),
	})
}
