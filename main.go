//go:build !exclude_wails

package main

import (
	"embed"
	"log"
	"os"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/gui"
	"github.com/wailsapp/wails/v2/pkg/application"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed frontend/dist
var assets embed.FS

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger := log.New(os.Stdout, "[mcp-hub-gui] ", log.LstdFlags)

	app := application.NewWithOptions(&options.App{
		Title:  "mcp-hub",
		Width:  1200,
		Height: 800,
		Assets: assets,
		Mac: &mac.Options{
			TitleBar: mac.TitleBarDefault(),
		},
	})

	g := gui.New(app, cfg, logger)
	app.Bind(g)

	if err := g.Start(); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}
	defer g.Stop()

	if err := app.Run(); err != nil {
		log.Fatalf("Failed to run app: %v", err)
	}
}
