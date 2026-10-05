package cmd

import (
	"encoding/json"
	"testing"

	versioncmd "github.com/slavkluev/ytr/internal/cmd/version"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
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
		{
			name: "Empty selection with jq", args: []string{"version", "--json=", "--jq", "."},
			code: ytrerrors.ExitUserError, stderr: []string{noFieldsDocument(versioncmd.VersionFields)},
		},
	})
}
