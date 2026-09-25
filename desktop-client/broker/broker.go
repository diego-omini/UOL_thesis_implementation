// Package broker defines application-owned boundaries for external brokerage integrations.
package broker

// This file defines the read-only connection and portfolio contract implemented by broker adapters.

import (
	"context"
	"wails/portfolio"
)

// ConnectionStatus describes whether a broker connection is ready for portfolio access.
type ConnectionStatus struct {
	Connected bool
	Message   string
}

// SessionState identifies the current stage of the broker authentication flow.
type SessionState string

const (
	SessionGatewayUnavailable           SessionState = "gateway_unavailable"
	SessionGatewayConfigurationRequired SessionState = "gateway_configuration_required"
	SessionGatewayStarting              SessionState = "gateway_starting"
	SessionAuthenticationRequired       SessionState = "authentication_required"
	SessionCompeting                    SessionState = "competing_session"
	SessionLiveAccount                  SessionState = "live_account"
	SessionAuthenticated                SessionState = "authenticated"
	SessionError                        SessionState = "error"
)

// SessionStatus describes gateway availability, authentication, and paper-account validation.
type SessionStatus struct {
	State            SessionState
	GatewayReachable bool
	Authenticated    bool
	Connected        bool
	Competing        bool
	Paper            bool
	PaperKnown       bool
	Message          string
}

// Ready reports whether broker data may be exposed to the application.
func (s SessionStatus) Ready() bool {
	return s.State == SessionAuthenticated && s.Authenticated && s.Connected && s.PaperKnown && s.Paper
}

// PortfolioProvider exposes the read-only broker operations used by the application.
type PortfolioProvider interface {
	CheckConnection(ctx context.Context) ConnectionStatus
	FetchPortfolio(ctx context.Context) (portfolio.Portfolio, error)
}

// InstrumentSearchProvider exposes broker-backed stock discovery without leaking broker DTOs.
type InstrumentSearchProvider interface {
	SearchStocks(ctx context.Context, query string) ([]portfolio.InstrumentSearchResult, error)
}

// StockDataProvider exposes current and historical stock market data.
type StockDataProvider interface {
	FetchStockSnapshot(ctx context.Context, contractID int64) (portfolio.StockSnapshot, error)
	FetchStockHistory(ctx context.Context, contractID int64, timeRange string) (portfolio.StockHistory, error)
}

// SessionProvider exposes lifecycle operations without exposing broker credentials.
type SessionProvider interface {
	CheckSession(ctx context.Context) SessionStatus
	KeepAlive(ctx context.Context) SessionStatus
	Logout(ctx context.Context) error
}
