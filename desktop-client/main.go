// Package main runs the Wails desktop client and exposes backend operations to its frontend.
package main

// main.go embeds the frontend assets and starts the configured Wails desktop application.

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// main constructs the application backend and starts the Wails runtime.
func main() {
	app, err := NewApp()
	if err != nil {
		println("Error:", err.Error())
		return
	}

	err = wails.Run(&options.App{
		Title:  "AI-assisted portfolio tracker",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
