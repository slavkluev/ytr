package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	ytrerrors "github.com/slavkluev/ytr/internal/errors"
)

// OrgType identifies which Tracker organization header must be sent.
type OrgType string

const (
	// OrgType360 uses the X-Org-Id header.
	OrgType360 OrgType = "360"

	// OrgTypeCloud uses the X-Cloud-Org-Id header.
	OrgTypeCloud OrgType = "cloud"
)

// Config holds the persistent ytr configuration loaded from config.yaml.
// The flat structure (no profiles) is intentional for v1 simplicity.
type Config struct {
	// Token is the OAuth token for Yandex Tracker API authentication.
	Token string `yaml:"token,omitempty"`

	// OrgID is the Tracker organization ID.
	OrgID string `yaml:"org_id,omitempty"`

	// OrgType determines which Tracker organization header to send.
	OrgType OrgType `yaml:"org_type,omitempty"`
}

// ResolvedAuth holds the resolved authentication credentials and their source.
// TokenSource indicates which tier provided the credentials.
type ResolvedAuth struct {
	// Token is the resolved OAuth token.
	Token string

	// OrgID is the resolved organization ID.
	OrgID string

	// OrgType determines which Tracker organization header to send.
	OrgType OrgType

	// TokenSource indicates where credentials came from: "flag", "env", or "config".
	TokenSource string
}

// Load reads the config file and returns the parsed Config.
// If the config file does not exist, it returns an empty Config with no error.
func Load() (*Config, error) {
	path, err := ConfigFilePath()
	if err != nil {
		return nil, fmt.Errorf("failed to determine config path: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

const (
	dirPermissions  = 0o700 // Owner read/write/execute for config directory.
	filePermissions = 0o600 // Owner read/write for config file.
)

// ParseOrgType validates and normalizes the organization type.
func ParseOrgType(value string) (OrgType, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case string(OrgType360):
		return OrgType360, nil
	case string(OrgTypeCloud):
		return OrgTypeCloud, nil
	default:
		return "", fmt.Errorf("invalid org-type %q", value)
	}
}

// Save writes the config to disk atomically with secure permissions.
// The config directory is created with 0700 if it does not exist.
// The file is written to a temp file first, then atomically renamed
// to prevent corruption from interrupted writes.
func Save(cfg *Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return fmt.Errorf("failed to determine config directory: %w", err)
	}

	if mkdirErr := os.MkdirAll(dir, dirPermissions); mkdirErr != nil {
		return fmt.Errorf("failed to create config directory: %w", mkdirErr)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	path, err := ConfigFilePath()
	if err != nil {
		return fmt.Errorf("failed to determine config path: %w", err)
	}

	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, filePermissions); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write config: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on rename failure; ignore error since
		// we are already returning a more important rename error.
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// ResolveAuth resolves authentication credentials using three-tier precedence:
// flags > env vars > config file. Flag overrides are strict: any partial or
// invalid auth flags are rejected. Environment variables are only used when the
// tier is complete. Config file auth is strict once reached so persisted
// partial credentials are surfaced to the user instead of being ignored.
func ResolveAuth(
	flagToken, flagOrgID, flagOrgType string,
) (*ResolvedAuth, error) {
	// Tier 1: Flags
	if hasCompleteAuth(flagToken, flagOrgID, flagOrgType) {
		return resolveAuthTier("flag", "", flagToken, flagOrgID, flagOrgType)
	}
	if hasAnyAuth(flagToken, flagOrgID, flagOrgType) {
		if flagOrgType != "" {
			if _, err := ParseOrgType(flagOrgType); err != nil {
				return nil, ytrerrors.NewUserError(
					err.Error(),
					invalidOrgTypeSuggestion("flag", ""),
				)
			}
		}
		return nil, ytrerrors.NewUserError(
			"incomplete flag authentication",
			incompleteAuthSuggestion("flag", ""),
		)
	}

	// Tier 2: Environment variables
	envToken := os.Getenv("YTR_TOKEN")
	envOrgID := os.Getenv("YTR_ORG_ID")
	envOrgType := os.Getenv("YTR_ORG_TYPE")
	if hasCompleteAuth(envToken, envOrgID, envOrgType) {
		return resolveAuthTier("env", "", envToken, envOrgID, envOrgType)
	}

	// Tier 3: Config file
	cfgPath, err := ConfigFilePath()
	if err != nil {
		return nil, ytrerrors.NewUserError(
			fmt.Sprintf("failed to determine config path: %v", err),
			"Set YTR_CONFIG_DIR or provide credentials via flags or environment variables",
		)
	}

	cfg, err := Load()
	if err != nil {
		return nil, ytrerrors.NewUserError(
			fmt.Sprintf("failed to load config: %v", err),
			fmt.Sprintf(
				"Fix or remove %s, or provide credentials via flags or environment variables",
				cfgPath,
			),
		)
	}
	if hasCompleteAuth(cfg.Token, cfg.OrgID, string(cfg.OrgType)) {
		return resolveAuthTier("config", cfgPath, cfg.Token, cfg.OrgID, string(cfg.OrgType))
	}
	if hasAnyAuth(cfg.Token, cfg.OrgID, string(cfg.OrgType)) {
		if cfg.OrgType != "" {
			if _, err := ParseOrgType(string(cfg.OrgType)); err != nil {
				return nil, ytrerrors.NewUserError(
					err.Error(),
					invalidOrgTypeSuggestion("config", cfgPath),
				)
			}
		}
		return nil, ytrerrors.NewUserError(
			"incomplete config authentication",
			incompleteAuthSuggestion("config", cfgPath),
		)
	}

	return nil, ytrerrors.NewAuthError(
		"not authenticated",
		"Run: ytr auth login\nOr set: export YTR_TOKEN=<token> YTR_ORG_ID=<org> YTR_ORG_TYPE=<360|cloud>",
	)
}

func hasAnyAuth(token, orgID, orgType string) bool {
	return token != "" || orgID != "" || orgType != ""
}

func hasCompleteAuth(token, orgID, orgType string) bool {
	return token != "" && orgID != "" && orgType != ""
}

func resolveAuthTier(source, configPath, token, orgID, orgTypeRaw string) (*ResolvedAuth, error) {
	orgType, err := ParseOrgType(orgTypeRaw)
	if err != nil {
		return nil, ytrerrors.NewUserError(
			err.Error(),
			invalidOrgTypeSuggestion(source, configPath),
		)
	}

	return &ResolvedAuth{
		Token:       token,
		OrgID:       orgID,
		OrgType:     orgType,
		TokenSource: source,
	}, nil
}

func invalidOrgTypeSuggestion(source, configPath string) string {
	switch source {
	case "flag":
		return "Use --org-type 360 or --org-type cloud"
	case "env":
		return "Set YTR_ORG_TYPE to 360 or cloud"
	case "config":
		return fmt.Sprintf(
			"Set org_type to 360 or cloud in %s, or run ytr auth login",
			configPath,
		)
	default:
		return "Use org-type 360 or cloud"
	}
}

func incompleteAuthSuggestion(source, configPath string) string {
	switch source {
	case "flag":
		return "Provide --token, --org-id, and --org-type together"
	case "config":
		return fmt.Sprintf(
			"Set token, org_id, and org_type in %s, or run ytr auth login",
			configPath,
		)
	default:
		return "Provide token, org_id, and org_type together"
	}
}
