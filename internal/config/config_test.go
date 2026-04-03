package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/harisbinrahat/mcp-hub/internal/config"
)

func TestLoadExpandsEnvAndDefaults(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("API_TOKEN", "token-from-os")

	configDir := filepath.Join(home, ".config", "mcp-hub")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(configDir, ".env"), []byte("FIXTURE_SECRET=secret-from-dotenv\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	configYAML := `
hub:
  port: 5001
servers:
  fixture:
    command: "${HOME}/bin/fixture"
    args: ["--token", "${API_TOKEN}", "--path", "~/workspace"]
    env:
      SECRET: "${FIXTURE_SECRET}"
    lazy: false
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	server := cfg.Servers["fixture"]
	if cfg.Hub.Host != "127.0.0.1" {
		t.Fatalf("expected default host, got %q", cfg.Hub.Host)
	}
	if cfg.Hub.Port != 5001 {
		t.Fatalf("expected configured port, got %d", cfg.Hub.Port)
	}
	if cfg.Hub.LogFile != filepath.Join(configDir, "hub.log") {
		t.Fatalf("expected default log path in config dir, got %q", cfg.Hub.LogFile)
	}
	if server.Command != filepath.Join(home, "bin", "fixture") {
		t.Fatalf("expected expanded command path, got %q", server.Command)
	}
	if got := server.Args[3]; got != filepath.Join(home, "workspace") {
		t.Fatalf("expected home-expanded arg, got %q", got)
	}
	if got := server.Env["SECRET"]; got != "secret-from-dotenv" {
		t.Fatalf("expected dotenv expansion, got %q", got)
	}
	if server.IsLazy() {
		t.Fatalf("expected lazy=false override to be respected")
	}
}
