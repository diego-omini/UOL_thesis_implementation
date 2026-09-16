// Package config defines application configuration types for external services and local settings.
package config

// This file loads and validates IBKR Client Portal Gateway connection settings.

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
	ibkrBaseURLEnv       = "IBKR_BASE_URL"
	ibkrSkipTLSVerifyEnv = "IBKR_SKIP_TLS_VERIFY"
	ibkrGatewayDirEnv    = "IBKR_GATEWAY_DIR"
	settingsDirectory    = "uol-portfolio-tracker"
	settingsFilename     = "settings.json"
)

// IBKRConfig contains connection settings for the IBKR paper trading client.
type IBKRConfig struct {
	BaseURL            string
	SkipTLSVerifyLocal bool
	GatewayDirectory   string
}

type localSettings struct {
	GatewayDirectory string `json:"gatewayDirectory"`
}

// AppConfig groups the configurable settings used by the desktop application.
type AppConfig struct {
	IBKR IBKRConfig
}

// DefaultConfig returns development defaults for the local IBKR Client Portal Gateway.
func DefaultConfig() AppConfig {
	return AppConfig{
		IBKR: IBKRConfig{
			BaseURL:            "https://localhost:5000/v1/api",
			SkipTLSVerifyLocal: true,
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

	if err := validateBaseURL(cfg.IBKR.BaseURL); err != nil {
		return AppConfig{}, fmt.Errorf("invalid %s: %w", ibkrBaseURLEnv, err)
	}

	return cfg, nil
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
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return filepath.Join(directory, settingsDirectory, settingsFilename), nil
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
