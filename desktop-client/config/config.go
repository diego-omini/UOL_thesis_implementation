// Package config defines application configuration types for external services and local settings.
package config

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
			BaseURL:            "https://localhost:5000/v1/api", // TODO: make this parametrizable per environment
			SkipTLSVerifyLocal: true,
		},
	}
}
