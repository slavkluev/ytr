package api

import (
	stderrors "errors"
	"net/http"
	"time"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

const httpTimeout = 30 * time.Second

type invalidAuthTransport struct {
	err error
}

func (t invalidAuthTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	if t.err == nil {
		return nil, stderrors.New("invalid authentication configuration")
	}

	return nil, t.err
}

// NewClient creates a new Yandex Tracker API client from resolved auth credentials.
// The client uses a 30-second HTTP timeout to prevent hanging on network issues.
func NewClient(auth *config.ResolvedAuth) *tracker.Client {
	if auth == nil {
		return newInvalidAuthClient(
			nil,
			ytrerrors.NewAuthError(
				"not authenticated",
				"Run: ytr auth login\nOr set: export YTR_TOKEN=<token> YTR_ORG_ID=<org> YTR_ORG_TYPE=<360|cloud>",
			),
		)
	}

	orgType, err := config.ParseOrgType(string(auth.OrgType))
	if err != nil {
		return newInvalidAuthClient(
			auth,
			ytrerrors.NewUserError(
				err.Error(),
				"Use org-type 360 or cloud, or rerun ytr auth login",
			),
		)
	}

	httpClient := &http.Client{
		Timeout:   httpTimeout,
		Transport: newDebugTransport(nil, auth.TokenSource),
	}

	opts := []tracker.Option{
		tracker.WithOAuthToken(auth.Token),
		tracker.WithHTTPClient(httpClient),
	}

	switch orgType {
	case config.OrgType360:
		opts = append(opts, tracker.WithOrgID(auth.OrgID))
	case config.OrgTypeCloud:
		opts = append(opts, tracker.WithCloudOrgID(auth.OrgID))
	}

	return tracker.NewClient(opts...)
}

func newInvalidAuthClient(auth *config.ResolvedAuth, err error) *tracker.Client {
	httpClient := &http.Client{
		Timeout:   httpTimeout,
		Transport: invalidAuthTransport{err: err},
	}

	opts := []tracker.Option{
		tracker.WithHTTPClient(httpClient),
	}
	if auth != nil && auth.Token != "" {
		opts = append(opts, tracker.WithOAuthToken(auth.Token))
	}

	return tracker.NewClient(opts...)
}
