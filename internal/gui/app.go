package gui

import (
	"context"
	"log"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/hub"
	"github.com/wailsapp/wails/v2/pkg/application"
	"gopkg.in/yaml.v3"
)

type App struct {
	instance *application.Application
	hub      *hub.Hub
	config   config.Config
	logger   *log.Logger
}

func New(app *application.Application, cfg config.Config, logger *log.Logger) *App {
	return &App{
		instance: app,
		config:   cfg,
		logger:   logger,
	}
}

func (a *App) Start() error {
	h := hub.New(a.config, a.logger)
	a.hub = h
	return h.StartEager(context.Background())
}

func (a *App) Stop() {
	if a.hub != nil {
		a.hub.Close()
	}
}

func (a *App) Status() map[string]interface{} {
	if a.hub == nil {
		return map[string]interface{}{}
	}
	status := a.hub.Status()
	return map[string]interface{}{
		"version": status.Version,
		"address": status.Address,
		"servers": status.Servers,
	}
}

func (a *App) ListServers() []map[string]interface{} {
	if a.hub == nil {
		return nil
	}
	status := a.hub.Status()
	result := make([]map[string]interface{}, len(status.Servers))
	for i, s := range status.Servers {
		result[i] = map[string]interface{}{
			"name":        s.Name,
			"mode":        s.Mode,
			"status":      s.Status,
			"pid":         s.PID,
			"clientCount": s.Clients,
			"restarts":    s.Restarts,
			"lastError":   s.LastError,
			"lazy":        s.Lazy,
			"lastPingMs":  s.LastPingMs,
			"lastPingAt":  s.LastPingAt,
		}
	}
	return result
}

func (a *App) ReloadConfig() error {
	if a.hub == nil {
		return nil
	}
	cfg, err := config.Load("")
	if err != nil {
		return err
	}
	a.config = cfg
	return a.hub.Reload(context.Background(), cfg)
}

func (a *App) SaveConfig(yamlContent string) error {
	if a.hub == nil {
		return nil
	}
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(yamlContent), &cfg); err != nil {
		return err
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	return a.hub.Reload(context.Background(), cfg)
}
