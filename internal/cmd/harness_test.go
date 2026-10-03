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

// harnessAuth are complete auth flags, which win over the environment and the
// config file.
var harnessAuth = []string{"--token=test-token", "--org-id=test-org", "--org-type=360"}

// runCLI runs args through the real root command and the real Tracker client,
// with a fake Tracker serving exchanges in place of the network. The config
// directory is a fresh one, so no credential comes from the developer's
// machine and a command that writes config, such as auth login or logout,
// never touches theirs. The run writes to a pipe, not a terminal.
func runCLI(t *testing.T, exchanges []faketracker.Exchange, args ...string) cliResult {
	t.Helper()

	return runCLIOn(t, output.Options{}, exchanges, args...)
}

// runCLIOn is runCLI writing to the terminal term describes, such as
// output.Options{TTY: true, Colors: true}. Only its terminal facts count: the
// output flags come from args.
func runCLIOn(t *testing.T, term output.Options, exchanges []faketracker.Exchange, args ...string) cliResult {
	t.Helper()

	return runAgainst(t, term, faketracker.New(t, exchanges), slices.Concat(harnessAuth, args))
}

// runSignedIn is runCLI against fake.
func runSignedIn(t *testing.T, fake *faketracker.Fake, args ...string) cliResult {
	t.Helper()

	return runAgainst(t, output.Options{}, fake, slices.Concat(harnessAuth, args))
}

// runSignedOut is runSignedIn with no credentials in the flags, the
// environment or the config directory, so a run that reaches auth fails there
// with exit 3.
func runSignedOut(t *testing.T, fake *faketracker.Fake, args ...string) cliResult {
	t.Helper()
	t.Setenv("YTR_TOKEN", "")
	t.Setenv("YTR_ORG_ID", "")
	t.Setenv("YTR_ORG_TYPE", "")

	return runAgainst(t, output.Options{}, fake, args)
}

func runAgainst(t *testing.T, term output.Options, fake *faketracker.Fake, argv []string) cliResult {
	t.Helper()
	t.Setenv("YTR_CONFIG_DIR", t.TempDir())

	var out, errOut bytes.Buffer
	code := execute(api.WithTransport(t.Context(), fake), term, argv, &out, &errOut)

	return cliResult{Code: code, Stdout: out.String(), Stderr: errOut.String(), Requests: fake.Requests()}
}
