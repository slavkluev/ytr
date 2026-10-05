package cmd

import (
	"testing"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
)

func TestQueueView(t *testing.T) {
	t.Parallel()

	const path = "/v3/queues/MYQUEUE"
	queue := trackerGET(path, `{"key": "MYQUEUE", "name": "My Queue", "description": "Queue for tracking tasks",
		"lead": {"id": "uid-a", "display": "Иван Петров"}, "defaultType": {"display": "Task", "name": "Задача"},
		"defaultPriority": {"display": "Normal", "key": "normal"}, "assignAuto": true, "allowExternals": false}`)
	byName := trackerGET(path, `{"key": "MYQUEUE", "name": "My Queue",
		"defaultType": {"name": "Bug", "key": "bug"}, "defaultPriority": {"name": "Critical", "key": "critical"}}`)
	byKey := trackerGET(path, `{"key": "MYQUEUE", "name": "My Queue",
		"defaultType": {"key": "task"}, "defaultPriority": {}}`)
	emptyDisplay := trackerGET(path, `{"key": "MYQUEUE", "name": "My Queue",
		"defaultType": {"display": "", "name": "Bug"}, "defaultPriority": {"display": "", "key": "critical"}}`)
	bare := trackerGET(path, `{"key": "MYQUEUE"}`)

	runLeafRows(t, []leafRow{
		{
			name: "Every field", args: []string{"queue", "view", "MYQUEUE"},
			exchanges: []faketracker.Exchange{queue},
			json: `{"key": "MYQUEUE", "name": "My Queue", "description": "Queue for tracking tasks",
				"lead": "Иван Петров", "leadId": "uid-a", "defaultType": "Task", "defaultPriority": "Normal",
				"assignAuto": true, "allowExternals": false}`,
		},
		{
			name: "Types by name", args: []string{"queue", "view", "MYQUEUE"},
			exchanges: []faketracker.Exchange{byName},
			json: `{"key": "MYQUEUE", "name": "My Queue", "leadId": "", "defaultType": "Bug",
				"defaultPriority": "Critical", "assignAuto": false, "allowExternals": false}`,
		},
		{
			name: "Types by key", args: []string{"queue", "view", "MYQUEUE"},
			exchanges: []faketracker.Exchange{byKey},
			json: `{"key": "MYQUEUE", "name": "My Queue", "leadId": "", "defaultType": "task",
				"assignAuto": false, "allowExternals": false}`,
		},
		{
			name: "An empty display", args: []string{"queue", "view", "MYQUEUE"},
			exchanges: []faketracker.Exchange{emptyDisplay},
			json: `{"key": "MYQUEUE", "name": "My Queue", "leadId": "", "assignAuto": false,
				"allowExternals": false}`,
		},
		{
			name: "A bare queue", args: []string{"queue", "view", "MYQUEUE"}, exchanges: []faketracker.Exchange{bare},
			json: `{"key": "MYQUEUE", "name": "", "leadId": "", "assignAuto": false, "allowExternals": false}`,
		},
		{
			name: "jq", args: []string{"queue", "view", "MYQUEUE", "--jq", ".leadId"},
			exchanges: []faketracker.Exchange{queue}, stdout: "uid-a\n",
		},
		{
			name: "Blank key", args: []string{"queue", "view", " "},
			code: ytrerrors.ExitUserError, stderr: []string{"invalid queue key: expected a non-empty value"},
		},
		notFoundRow("/v3/queues/NOEXIST", "queue", "view", "NOEXIST", "--json", "key"),
	})
}
