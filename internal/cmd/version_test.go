package cmd

import (
	"encoding/json"
	"testing"

	ver "github.com/slavkluev/ytr/internal/version"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	info := ver.Get()
	doc, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("encoding the build info: %v", err)
	}

	runLeafRows(t, []leafRow{
		{name: "Every field", args: []string{"version"}, json: string(doc)},
		{name: "JSON", args: []string{"version", "--json", "version"}, json: `{"version": "` + info.Version + `"}`},
		{name: "jq of the whole document", args: []string{"version", "--jq", "."}, json: string(doc)},
	})
}
