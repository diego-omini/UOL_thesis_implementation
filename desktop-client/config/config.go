// Package config defines application configuration types for external services and local settings.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	ibkrBaseURLEnv       = "IBKR_BASE_URL"
	ibkrSkipTLSVerifyEnv = "IBKR_SKIP_TLS_VERIFY"
)

// IBKRConfig contains connection settings for the IBKR paper trading client.
type IBKRConfig struct {
	BaseURL            string
	SkipTLSVerifyLocal bool
}

// AppConfig groups the configurable settings used by the desktop application.
type AppConfig struct {
	IBKR IBKRConfig
}

// DefaultConfig returns development defaults for the local IBKR Client Portal Gateway
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
