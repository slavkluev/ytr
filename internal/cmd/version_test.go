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
		{
			name: "Text", args: []string{"version"},
			stdout: "ytr version " + info.Version + "\ncommit: " + info.Commit + "\ndate: " + info.Date +
				"\ngo: " + info.GoVersion + "\nos/arch: " + info.OS + "/" + info.Arch + "\n",
		},
		{name: "Quiet", args: []string{"version", "--quiet"}, stdout: info.Version + "\n"},
		{
			name: "JSON of every field", args: []string{"version", "--json", "version,commit,date,goVersion,os,arch"},
			json: string(doc),
		},
		{name: "jq of the whole document", args: []string{"version", "--jq", "."}, json: string(doc)},
	})
}
