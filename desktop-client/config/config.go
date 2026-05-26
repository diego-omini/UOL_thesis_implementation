// Package config defines application configuration types for external services and local settings.
package config

// IBKRConfig contains connection settings for the IBKR paper trading client.
type IBKRConfig struct {
	Host     string
	Port     int
	ClientID int
}

// AppConfig groups the configurable settings used by the desktop application.
type AppConfig struct {
	IBKR IBKRConfig
}
