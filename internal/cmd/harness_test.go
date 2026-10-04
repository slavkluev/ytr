package cmd

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	"github.com/slavkluev/ytr/internal/faketracker"
	"github.com/slavkluev/ytr/internal/output"
)

// cliResult is what one ytr invocation produced, plus every request Tracker
// would have received from it and the config directory it ran with.
type cliResult struct {
	Code      int
	Stdout    string
	Stderr    string
	Requests  []faketracker.Request
	ConfigDir string
}

// cliInput is what a run reads besides its arguments and Tracker: the terminal
// term describes, such as output.Options{TTY: true, Colors: true}, of which
// only the terminal facts count; stdin; environment variables beyond
// YTR_CONFIG_DIR; and the config.yaml its config directory starts with.
type cliInput struct {
	term   output.Options
	stdin  string
	env    map[string]string
	config string
}

// harnessAuth are complete auth flags, which win over the environment and the
// config file.
var harnessAuth = []string{"--token=test-token", "--org-id=test-org", "--org-type=360"}

// runCLI runs args through the real root command and the real Tracker client,
// signed in by harnessAuth, with a fake Tracker serving exchanges in place of
// the network. The run writes to a pipe, not a terminal, and its stdin is
// empty.
func runCLI(t *testing.T, exchanges []faketracker.Exchange, args ...string) cliResult {
	t.Helper()

	return runSignedIn(t, faketracker.New(t, exchanges), args...)
}

// runSignedIn is runCLI against fake.
func runSignedIn(t *testing.T, fake *faketracker.Fake, args ...string) cliResult {
	t.Helper()

	return runAgainst(t, cliInput{}, fake, slices.Concat(harnessAuth, args))
}

// runSignedOut is runSignedIn without the auth flags. The run's environment and
// config directory hold no credentials either, so a run that reaches auth fails
// there with exit 3.
func runSignedOut(t *testing.T, fake *faketracker.Fake, args ...string) cliResult {
	t.Helper()

	return runAgainst(t, cliInput{}, fake, args)
}

// runAgainst runs argv against fake with a config directory of its own and an
// environment that holds only YTR_CONFIG_DIR, naming that directory, and
// in.env. Both reach the run through its context, never the process, so no
// credential or config comes from the developer's machine, a command that
// writes config never touches theirs, and parallel runs share no file.
func runAgainst(t *testing.T, in cliInput, fake *faketracker.Fake, argv []string) cliResult {
	t.Helper()

	dir := t.TempDir()
	if in.config != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(in.config), 0o600); err != nil {
			t.Fatalf("writing the starting config: %v", err)
		}
	}

	env := maps.Clone(in.env)
	if env == nil {
		env = make(map[string]string, 1)
	}
	env["YTR_CONFIG_DIR"] = dir
	lookup := func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}
	ctx := config.WithEnv(api.WithTransport(t.Context(), fake), lookup)

	var out, errOut bytes.Buffer
	code := execute(ctx, in.term, argv, strings.NewReader(in.stdin), &out, &errOut)

	return cliResult{
		Code: code, Stdout: out.String(), Stderr: errOut.String(), Requests: fake.Requests(), ConfigDir: dir,
	}
}
