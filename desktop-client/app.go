package main

import (
	"context"
	"fmt"
)

// App is the Wails application object whose exported methods can be called from the frontend.
type App struct {
	ctx context.Context
}

// NewApp creates the Wails application object.
func NewApp() *App {
	return &App{}
}

// startup stores the Wails runtime context when the application starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Greet returns a greeting for the given name.
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
