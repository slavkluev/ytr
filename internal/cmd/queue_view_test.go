package cmd

import (
	"testing"

	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

func TestQueueView(t *testing.T) {
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
	all := "key,name,description,lead,leadId,defaultType,defaultPriority,assignAuto,allowExternals"

	runLeafRows(t, []leafRow{
		{
			name: "JSON", args: []string{"queue", "view", "MYQUEUE", "--json", all},
			exchanges: []faketracker.Exchange{queue},
			json: `{"key": "MYQUEUE", "name": "My Queue", "description": "Queue for tracking tasks",
				"lead": "Иван Петров", "leadId": "uid-a", "defaultType": "Task", "defaultPriority": "Normal",
				"assignAuto": true, "allowExternals": false}`,
		},
		{
			name: "JSON names types by name", args: []string{"queue", "view", "MYQUEUE", "--json", all},
			exchanges: []faketracker.Exchange{byName},
			json: `{"key": "MYQUEUE", "name": "My Queue", "leadId": "", "defaultType": "Bug",
				"defaultPriority": "Critical", "assignAuto": false, "allowExternals": false}`,
		},
		{
			name: "JSON names types by key", args: []string{"queue", "view", "MYQUEUE", "--json", all},
			exchanges: []faketracker.Exchange{byKey},
			json: `{"key": "MYQUEUE", "name": "My Queue", "leadId": "", "defaultType": "task",
				"assignAuto": false, "allowExternals": false}`,
		},
		{
			name: "Card", args: []string{"queue", "view", "MYQUEUE"}, exchanges: []faketracker.Exchange{queue},
			stdout: "Key\tMYQUEUE\nName\tMy Queue\nLead\tИван Петров\nDefault Type\tTask\nDefault Priority\tNormal\n" +
				"\nDescription:\n  Queue for tracking tasks\n",
		},
		{
			name: "Card names types by name", args: []string{"queue", "view", "MYQUEUE"},
			exchanges: []faketracker.Exchange{byName},
			stdout:    "Key\tMYQUEUE\nName\tMy Queue\nLead\t-\nDefault Type\tBug\nDefault Priority\tCritical\n",
		},
		{
			name: "Card names types by key", args: []string{"queue", "view", "MYQUEUE"},
			exchanges: []faketracker.Exchange{byKey},
			stdout:    "Key\tMYQUEUE\nName\tMy Queue\nLead\t-\nDefault Type\ttask\nDefault Priority\t-\n",
		},
		{
			name: "JSON keeps an empty display", args: []string{"queue", "view", "MYQUEUE", "--json", all},
			exchanges: []faketracker.Exchange{emptyDisplay},
			json: `{"key": "MYQUEUE", "name": "My Queue", "leadId": "", "assignAuto": false,
				"allowExternals": false}`,
		},
		{
			name: "Card keeps an empty display", args: []string{"queue", "view", "MYQUEUE"},
			exchanges: []faketracker.Exchange{emptyDisplay},
			stdout:    "Key\tMYQUEUE\nName\tMy Queue\nLead\t-\nDefault Type\t-\nDefault Priority\t-\n",
		},
		{
			name: "Card of a bare queue", args: []string{"queue", "view", "MYQUEUE"},
			exchanges: []faketracker.Exchange{bare},
			stdout:    "Key\tMYQUEUE\nName\t-\nLead\t-\nDefault Type\t-\nDefault Priority\t-\n",
		},
		{
			name: "TTY", args: []string{"queue", "view", "MYQUEUE"}, term: output.Options{TTY: true, Colors: true},
			exchanges: []faketracker.Exchange{queue},
			holds: []string{"Key:  MYQUEUE\nName:  My Queue\nLead:  Иван Петров\nDefault Type:  Task\n" +
				"Default Priority:  Normal\n\nDescription:\n  Queue for tracking tasks\n"},
		},
		{
			name: "Quiet", args: []string{"queue", "view", "MYQUEUE", "--quiet"},
			exchanges: []faketracker.Exchange{queue}, stdout: "MYQUEUE\n",
		},
		{
			name: "jq", args: []string{"queue", "view", "MYQUEUE", "--jq", ".leadId"},
			exchanges: []faketracker.Exchange{queue}, stdout: "uid-a\n",
		},
		{
			name: "Any arg", args: []string{"queue", "view", "q", "--quiet"},
			exchanges: []faketracker.Exchange{trackerGET("/v3/queues/q", `{"key": "Q"}`)}, stdout: "Q\n",
		},
		fieldHintRow("queue view", []string{"MYQUEUE"}, "key", "name", "description", "lead", "leadId",
			"defaultType", "defaultPriority", "assignAuto", "allowExternals"),
		notFoundRow("/v3/queues/NOEXIST", "queue", "view", "NOEXIST", "--json", "key"),
		helpRow("queue view", "JSON FIELDS\n  key, name, description, lead, leadId, defaultType, defaultPriority, "+
			"assignAuto, allowExternals\n\n"+
			"SEE ALSO\n  ytr queue list    - List queues\n  ytr issue list    - List issues in a queue\n"),
	})
}
