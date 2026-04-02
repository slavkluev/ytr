package api_test

import (
	"context"
	"errors"
	"testing"

	"github.com/slavkluev/ytr/internal/api"
	"github.com/slavkluev/ytr/internal/config"
	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name               string
		orgType            config.OrgType
		wantOrgHeader      string
		wantCloudOrgHeader string
	}{
		{
			name:          "yandex360",
			orgType:       config.OrgType360,
			wantOrgHeader: "test-org",
		},
		{
			name:               "cloud",
			orgType:            config.OrgTypeCloud,
			wantCloudOrgHeader: "test-org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &config.ResolvedAuth{
				Token:       "test-token",
				OrgID:       "test-org",
				OrgType:     tt.orgType,
				TokenSource: "flag",
			}

			client := api.NewClient(auth)
			if client == nil {
				t.Fatal("NewClient() returned nil")
			}

			req, err := client.NewRequest("GET", "v3/myself", nil)
			if err != nil {
				t.Fatalf("NewRequest() returned error: %v", err)
			}

			if got := req.Header.Get("Authorization"); got != "OAuth test-token" {
				t.Errorf("Authorization = %q, want %q", got, "OAuth test-token")
			}
			if got := req.Header.Get("X-Org-Id"); got != tt.wantOrgHeader {
				t.Errorf("X-Org-Id = %q, want %q", got, tt.wantOrgHeader)
			}
			if got := req.Header.Get("X-Cloud-Org-Id"); got != tt.wantCloudOrgHeader {
				t.Errorf("X-Cloud-Org-Id = %q, want %q", got, tt.wantCloudOrgHeader)
			}
		})
	}
}

func TestNewClient_InvalidAuthReturnsExitError(t *testing.T) {
	tests := []struct {
		name     string
		auth     *config.ResolvedAuth
		wantCode string
	}{
		{
			name:     "nil auth",
			auth:     nil,
			wantCode: "auth_error",
		},
		{
			name: "empty org type",
			auth: &config.ResolvedAuth{
				Token:       "test-token",
				OrgID:       "test-org",
				TokenSource: "flag",
			},
			wantCode: "user_error",
		},
		{
			name: "unsupported org type",
			auth: &config.ResolvedAuth{
				Token:       "test-token",
				OrgID:       "test-org",
				OrgType:     config.OrgType("bogus"),
				TokenSource: "flag",
			},
			wantCode: "user_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := api.NewClient(tt.auth)
			if client == nil {
				t.Fatal("NewClient() returned nil")
			}

			req, err := client.NewRequest("GET", "v3/myself", nil)
			if err != nil {
				t.Fatalf("NewRequest() returned error: %v", err)
			}

			_, err = client.Do(context.Background(), req, nil)
			if err == nil {
				t.Fatal("Do() should return error for invalid auth")
			}

			exitErr := &ytrerrors.ExitError{}
			if !errors.As(err, &exitErr) {
				t.Fatalf("error type = %T, want wrapped *ExitError", err)
			}
			if exitErr.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", exitErr.Code, tt.wantCode)
			}
		})
	}
}
