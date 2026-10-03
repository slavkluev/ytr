package cmd

import (
	"bytes"
	"slices"
	"testing"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

// cliResult is what one ytr invocation produced, plus every request Tracker
// would have received from it.
type cliResult struct {
	Code     int
	Stdout   string
	Stderr   string
	Requests []faketracker.Request
}

// runCLI runs args through the real root command and the real Tracker client,
// with a fake Tracker serving exchanges in place of the network. Complete auth
// flags win over the environment and the config file, and the config directory
// is a fresh one, so no credential comes from the developer's machine and a
// command that writes config, such as auth login or logout, never touches theirs.
func runCLI(t *testing.T, exchanges []faketracker.Exchange, args ...string) cliResult {
	t.Helper()
	t.Setenv("YTR_CONFIG_DIR", t.TempDir())

	fake := faketracker.New(t, exchanges)
	argv := slices.Concat([]string{"--token=test-token", "--org-id=test-org", "--org-type=360"}, args)

	var out, errOut bytes.Buffer
	code := execute(api.WithTransport(t.Context(), fake), output.Options{}, argv, &out, &errOut)

	return cliResult{Code: code, Stdout: out.String(), Stderr: errOut.String(), Requests: fake.Requests()}
}
