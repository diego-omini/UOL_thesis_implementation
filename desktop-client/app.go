package main

import (
	"context"
	"fmt"
	"time"
	"wails/analytics"
	"wails/broker"
	"wails/config"
	"wails/ibkr"
)

// App is the Wails application object whose exported methods can be called from the frontend.
type App struct {
	ctx             context.Context
	cfg             config.AppConfig
	portfolioBroker broker.PortfolioProvider
}

// NewApp creates the Wails application object.
func NewApp() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load application configuration: %w", err)
	}

	return &App{
		cfg: cfg,
		portfolioBroker: ibkr.NewClient(
			cfg.IBKR.BaseURL,
			cfg.IBKR.SkipTLSVerifyLocal,
		),
	}, nil
}

// startup stores the Wails runtime context when the application starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// *************************************************
// Expose data needed by the frontend
// *************************************************

func (a *App) CheckConnection() ConnectionStatusView {
	ctx := a.ctx

	if ctx == nil {
		ctx = context.Background()
	}

	status := a.portfolioBroker.CheckConnection(ctx)

	return toConnectionStatusView(status.Connected, status.Message)
}

func (a *App) GetPortfolio() (PortfolioView, error) {
	ctx := a.ctx

	if ctx == nil {
		ctx = context.Background()
	}

	portfolioSnapshot, err := a.portfolioBroker.FetchPortfolio(ctx)

	if err != nil {
		return PortfolioView{}, fmt.Errorf("failed to fetch portfolio: %w", err)
	}

	portfolioAnalytics := analytics.Calculate(portfolioSnapshot, time.Now())
	return toPortfolioView(portfolioSnapshot, portfolioAnalytics), nil
}
