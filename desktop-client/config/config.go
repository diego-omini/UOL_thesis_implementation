// Package config defines application configuration types for external services and local settings.
package config

// This file loads and validates external-service and local application settings.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	ibkrBaseURLEnv        = "IBKR_BASE_URL"
	ibkrSkipTLSVerifyEnv  = "IBKR_SKIP_TLS_VERIFY"
	ibkrGatewayDirEnv     = "IBKR_GATEWAY_DIR"
	aiProviderEnv         = "AI_PROVIDER"
	aiBaseURLEnv          = "AI_BASE_URL"
	aiModelEnv            = "AI_MODEL"
	ollamaAPIKeyEnv       = "OLLAMA_API_KEY"
	aiSearchBaseURLEnv    = "AI_SEARCH_BASE_URL"
	aiSearchDomainsEnv    = "AI_SEARCH_ALLOWED_DOMAINS"
	aiSearchMaxResultsEnv = "AI_SEARCH_MAX_RESULTS"
	settingsDirectory     = "uol-portfolio-tracker"
	settingsFilename      = "settings.json"
)

// IBKRConfig contains connection settings for the IBKR paper trading client.
type IBKRConfig struct {
	BaseURL            string
	SkipTLSVerifyLocal bool
	GatewayDirectory   string
}

// AIConfig contains provider-neutral selection and model connection settings.
type AIConfig struct {
	Provider             string
	BaseURL              string
	Model                string
	SearchAPIKey         string
	SearchBaseURL        string
	SearchAllowedDomains []string
	SearchMaxResults     int
}

type localSettings struct {
	GatewayDirectory string `json:"gatewayDirectory"`
}

// AppConfig groups the configurable settings used by the desktop application.
type AppConfig struct {
	IBKR IBKRConfig
	AI   AIConfig
}

// DefaultConfig returns development defaults for IBKR, local inference, and web research.
func DefaultConfig() AppConfig {
	return AppConfig{
		IBKR: IBKRConfig{
			BaseURL:            "https://localhost:5000/v1/api",
			SkipTLSVerifyLocal: true,
		},
		AI: AIConfig{
			Provider:      "ollama",
			BaseURL:       "http://localhost:11434/api",
			Model:         "llama3.2",
			SearchBaseURL: "https://ollama.com/api",
			SearchAllowedDomains: []string{
				"sec.gov", "finra.org", "nasdaq.com", "nyse.com",
				"reuters.com", "ft.com", "bloomberg.com", "morningstar.com",
				"ishares.com", "vanguard.com", "ssga.com", "invesco.com", "fidelity.com",
			},
			SearchMaxResults: 5,
		},
	}
}

// Load returns application configuration with optional environment overrides.
func Load() (AppConfig, error) {
	cfg := DefaultConfig()
	if value, ok := os.LookupEnv(ibkrGatewayDirEnv); ok {
		cfg.IBKR.GatewayDirectory = strings.TrimSpace(value)
	} else {
		settings, err := loadLocalSettings()
		if err != nil {
			return AppConfig{}, fmt.Errorf("failed to load local settings: %w", err)
		}
		cfg.IBKR.GatewayDirectory = settings.GatewayDirectory
	}

	if value, ok := os.LookupEnv(ibkrBaseURLEnv); ok {
		cfg.IBKR.BaseURL = strings.TrimSpace(value)
	}

	if value, ok := os.LookupEnv(ibkrSkipTLSVerifyEnv); ok {
		skipTLSVerify, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return AppConfig{}, fmt.Errorf("invalid %s value %q: %w", ibkrSkipTLSVerifyEnv, value, err)
		}

		cfg.IBKR.SkipTLSVerifyLocal = skipTLSVerify
	}
	if value, ok := os.LookupEnv(aiProviderEnv); ok {
		cfg.AI.Provider = strings.ToLower(strings.TrimSpace(value))
	}
	if value, ok := os.LookupEnv(aiBaseURLEnv); ok {
		cfg.AI.BaseURL = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv(aiModelEnv); ok {
		cfg.AI.Model = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv(ollamaAPIKeyEnv); ok {
		cfg.AI.SearchAPIKey = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv(aiSearchBaseURLEnv); ok {
		cfg.AI.SearchBaseURL = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv(aiSearchDomainsEnv); ok {
		cfg.AI.SearchAllowedDomains = splitNonEmpty(value)
	}
	if value, ok := os.LookupEnv(aiSearchMaxResultsEnv); ok {
		maximum, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return AppConfig{}, fmt.Errorf("invalid %s value %q: %w", aiSearchMaxResultsEnv, value, err)
		}
		cfg.AI.SearchMaxResults = maximum
	}

	if err := validateBaseURL(cfg.IBKR.BaseURL); err != nil {
		return AppConfig{}, fmt.Errorf("invalid %s: %w", ibkrBaseURLEnv, err)
	}
	if cfg.AI.Provider == "" {
		return AppConfig{}, fmt.Errorf("%s must not be empty", aiProviderEnv)
	}
	if err := validateBaseURL(cfg.AI.BaseURL); err != nil {
		return AppConfig{}, fmt.Errorf("invalid %s: %w", aiBaseURLEnv, err)
	}
	if cfg.AI.Model == "" {
		return AppConfig{}, fmt.Errorf("%s must not be empty", aiModelEnv)
	}
	if err := validateBaseURL(cfg.AI.SearchBaseURL); err != nil {
		return AppConfig{}, fmt.Errorf("invalid %s: %w", aiSearchBaseURLEnv, err)
	}
	if len(cfg.AI.SearchAllowedDomains) == 0 {
		return AppConfig{}, fmt.Errorf("%s must contain at least one domain", aiSearchDomainsEnv)
	}
	if cfg.AI.SearchMaxResults < 1 || cfg.AI.SearchMaxResults > 10 {
		return AppConfig{}, fmt.Errorf("%s must be between 1 and 10", aiSearchMaxResultsEnv)
	}

	return cfg, nil
}

func splitNonEmpty(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// SaveGatewayDirectory persists the non-sensitive location of the local Client Portal Gateway.
func SaveGatewayDirectory(directory string) error {
	path, err := localSettingsPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	contents, err := json.MarshalIndent(localSettings{GatewayDirectory: filepath.Clean(directory)}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}

	if err := os.WriteFile(path, contents, 0600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	return nil
}

func loadLocalSettings() (localSettings, error) {
	path, err := localSettingsPath()
	if err != nil {
		return localSettings{}, err
	}

	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return localSettings{}, nil
	}
	if err != nil {
		return localSettings{}, err
	}

	var settings localSettings
	if err := json.Unmarshal(contents, &settings); err != nil {
		return localSettings{}, fmt.Errorf("decode %s: %w", path, err)
	}

	return settings, nil
}

func localSettingsPath() (string, error) {
	directory, err := DataDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, settingsFilename), nil
}

// DataDirectory returns the application-owned directory for local settings and persistence.
func DataDirectory() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return filepath.Join(directory, settingsDirectory), nil
}

// validateBaseURL requires an absolute HTTP or HTTPS gateway URL.
func validateBaseURL(value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return err
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}

	if parsed.Host == "" {
		return fmt.Errorf("host is required")
	}

	return nil
}
