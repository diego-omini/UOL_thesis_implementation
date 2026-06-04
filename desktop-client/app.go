package main

import (
	"context"
	"wails/config"
	"wails/ibkr"
)

// App is the Wails application object whose exported methods can be called from the frontend.
type App struct {
	ctx context.Context
	cfg config.AppConfig
}

// NewApp creates the Wails application object.
func NewApp() *App {
	return &App{
		cfg: config.DefaultConfig(),
	}
}

// startup stores the Wails runtime context when the application starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) CheckIBKRConnection() ibkr.ConnectionStatus {
	ctx := a.ctx

	if ctx == nil {
		ctx = context.Background()
	}

	client := ibkr.NewClient(a.cfg.IBKR.BaseURL, a.cfg.IBKR.SkipTLSVerifyLocal)

	return client.CheckConnection(ctx)
}
