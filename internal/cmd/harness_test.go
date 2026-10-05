package cmd

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
	"github.com/slavkluev/ytr/internal/faketracker"
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

// cliInput is what a run reads besides its arguments and Tracker: stdin,
// environment variables beyond YTR_CONFIG_DIR, and the config.yaml its config
// directory starts with.
type cliInput struct {
	stdin  string
	env    map[string]string
	config string
}

// harnessAuth are complete auth flags, which win over the environment and the
// config file.
var harnessAuth = []string{"--token=test-token", "--org-id=test-org", "--org-type=360"}

// runCLI runs args through the real root command and the real Tracker client,
// signed in by harnessAuth, with a fake Tracker serving exchanges in place of
// the network. Its stdin is empty.
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
//
// Every harness run passes through here, so this is where each one is held to
// assertOneDocument.
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
	code := execute(ctx, argv, strings.NewReader(in.stdin), &out, &errOut)

	res := cliResult{
		Code: code, Stdout: out.String(), Stderr: errOut.String(), Requests: fake.Requests(), ConfigDir: dir,
	}
	assertOneDocument(t, argv, res)

	return res
}

// assertOneDocument fails t unless stdout and stderr of the run of argv, read
// together without the --debug lines, are exactly one JSON document: the error
// document on stderr after a failure, the result on stdout after a success. A
// caller that captures both streams merged can then parse whatever it got.
//
// A successful run that asked for text prints text instead; see asksForText.
func assertOneDocument(t *testing.T, argv []string, res cliResult) {
	t.Helper()

	label := commandLine(argv)
	stderr := withoutDebugLines(res.Stderr)

	if res.Code != ytrerrors.ExitSuccess {
		assertEmpty(t, label+": stdout of a failed run", res.Stdout)
		decodeErrorDocument(t, label, stderr)

		return
	}

	if asksForText(argv) {
		return
	}

	assertEmpty(t, label+": stderr of a successful run", stderr)
	if strings.Count(res.Stdout, "\n") != 1 || !strings.HasSuffix(res.Stdout, "\n") ||
		!json.Valid([]byte(res.Stdout)) {
		t.Errorf("%s: stdout = %q, want exactly one line of JSON", label, res.Stdout)
	}
}

// asksForText reports whether argv asks for one of the outputs that stay text
// when the run succeeds: help or a --jq stream.
func asksForText(argv []string) bool {
	command := ""
	for _, arg := range argv {
		switch {
		case arg == "--":
			return false
		case arg == "--help", arg == "-h", arg == "--jq", strings.HasPrefix(arg, "--jq="):
			return true
		case command == "" && !strings.HasPrefix(arg, "-"):
			command = arg
		}
	}

	return command == helpCommandName
}
