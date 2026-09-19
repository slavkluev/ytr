package queue

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/testutil"
)

// recordingTransport answers every request with an empty JSON body of the
// given shape and records the request line, so no request leaves the process.
type recordingTransport struct {
	body     string
	requests []string
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	line := req.Method + " " + strings.TrimPrefix(req.URL.Path, "/")
	if req.URL.RawQuery != "" {
		line += "?" + req.URL.RawQuery
	}
	rt.requests = append(rt.requests, line)

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(rt.body)),
		Request:    req,
	}, nil
}

// assertContextRequest runs call against a trackerContextClient whose
// transport answers with body, and checks the one request it sends.
func assertContextRequest(t *testing.T, body, want string, call func(queueContextClient) error) {
	t.Helper()

	rt := &recordingTransport{body: body}
	client := trackerContextClient{
		client: tracker.NewClient(tracker.WithHTTPClient(&http.Client{Transport: rt})),
	}

	if err := call(client); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rt.requests) != 1 || rt.requests[0] != want {
		t.Errorf("requests = %q, want [%q]", rt.requests, want)
	}
}

func TestTrackerContextClientRequests(t *testing.T) {
	testutil.ResetOutputFlags(t)
	ctx := t.Context()

	assertContextRequest(t, `{}`, "GET v3/queues/MTP?expand=issueTypesConfig", func(c queueContextClient) error {
		_, _, err := c.GetQueue(ctx, "MTP", &tracker.QueueGetOptions{Expand: "issueTypesConfig"})
		return err
	})
	assertContextRequest(t, `{}`, "GET v3/workflows/W207", func(c queueContextClient) error {
		_, _, err := c.GetWorkflow(ctx, "W207")
		return err
	})
	assertContextRequest(t, `[]`, "GET v3/queues/MTP/components?fields=name", func(c queueContextClient) error {
		_, _, err := c.ListComponents(ctx, "MTP", &tracker.QueueComponentsListOptions{Fields: "name"})
		return err
	})
	assertContextRequest(t, `[]`, "GET v3/queues/MTP/fields", func(c queueContextClient) error {
		_, _, err := c.ListQueueFields(ctx, "MTP")
		return err
	})
	assertContextRequest(t, `[]`, "GET v3/queues/MTP/localFields", func(c queueContextClient) error {
		_, _, err := c.ListLocalFields(ctx, "MTP")
		return err
	})
	assertContextRequest(t, `[]`, "GET v3/fields", func(c queueContextClient) error {
		_, _, err := c.ListGlobalFields(ctx)
		return err
	})
}
