package auth

import (
	"errors"
	"net/http"
	"testing"

	"github.com/slavkluev/go-yandex-tracker/tracker"

	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

func trackerError(status int, message string) error {
	req, _ := http.NewRequest(http.MethodGet, "https://api.tracker.yandex.net/v3/myself", nil)

	return &tracker.ErrorResponse{
		Response:      &http.Response{StatusCode: status, Request: req},
		ErrorMessages: []string{message},
	}
}

func failedAs(org360, cloud error) *orgTypeDetectionError {
	return &orgTypeDetectionError{Failures: []orgTypeAttemptFailure{
		{OrgType: config.OrgType360, Err: org360},
		{OrgType: config.OrgTypeCloud, Err: cloud},
	}}
}

// A transport failure never reaches the fake Tracker as a response, so the
// classification of detection failures is tested here, over the errors the
// client returns.
func TestClassifyOrgTypeDetectionError(t *testing.T) {
	timeout := errors.New("dial tcp api.tracker.yandex.net: i/o timeout")

	cases := []struct {
		name                      string
		err                       *orgTypeDetectionError
		code, message, suggestion string
	}{
		{
			name: "transport failures", err: failedAs(timeout, timeout), code: ytrerrors.CodeUserError,
			message: "failed to detect organization type: 360: dial tcp api.tracker.yandex.net: i/o timeout; " +
				"cloud: dial tcp api.tracker.yandex.net: i/o timeout",
			suggestion: "Check network connectivity and retry",
		},
		{
			name:       "a transport failure and a refusal",
			err:        failedAs(timeout, trackerError(http.StatusForbidden, "denied")),
			code:       ytrerrors.CodeUserError,
			suggestion: "Review the reported Tracker error details and retry",
		},
		{
			name: "both refused", err: failedAs(
				trackerError(http.StatusForbidden, "360 denied"), trackerError(http.StatusForbidden, "cloud denied")),
			code: ytrerrors.CodeAuthError, suggestion: "Check your token and access to the Tracker organization",
		},
		{
			name: "both not found",
			err: failedAs(
				trackerError(http.StatusNotFound, "360 missing"), trackerError(http.StatusNotFound, "cloud missing")),
			code:       ytrerrors.CodeNotFound,
			suggestion: "Check your organization ID and access to the Tracker organization",
		},
		{
			name: "both rate limited", err: failedAs(
				trackerError(http.StatusTooManyRequests, "slow"), trackerError(http.StatusTooManyRequests, "slow")),
			code: ytrerrors.CodeRateLimited, suggestion: "Wait and retry",
		},
		{
			name: "refused differently", err: failedAs(
				trackerError(http.StatusForbidden, "denied"), trackerError(http.StatusNotFound, "missing")),
			code: ytrerrors.CodeUserError, suggestion: "Retry with --org-type 360 or --org-type cloud",
		},
		{
			name: "server errors", err: failedAs(
				trackerError(http.StatusInternalServerError, "down"), trackerError(http.StatusBadGateway, "down")),
			code: ytrerrors.CodeUserError, suggestion: "Retry later",
		},
		{
			name: "no failures", err: nil, code: ytrerrors.CodeUserError,
			message: "failed to detect organization type", suggestion: "Retry with --org-type 360 or --org-type cloud",
		},
	}

	for _, c := range cases {
		var exitErr *ytrerrors.ExitError
		if !errors.As(classifyOrgTypeDetectionError(c.err), &exitErr) {
			t.Fatalf("%s: classified as %T, want an ExitError", c.name, exitErr)
		}
		if exitErr.Code != c.code || exitErr.Suggestion != c.suggestion {
			t.Errorf("%s: code %q, suggestion %q, want %q, %q",
				c.name, exitErr.Code, exitErr.Suggestion, c.code, c.suggestion)
		}
		if c.message != "" && exitErr.Message != c.message {
			t.Errorf("%s: message %q, want %q", c.name, exitErr.Message, c.message)
		}
	}
}
