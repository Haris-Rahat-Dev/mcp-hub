package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/harisbinrahat/mcp-hub/internal/config"
)

func TestGenerateClientArtifactsIncludesBuiltinClients(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := config.Config{
		Path: filepath.Join(home, ".config", "mcp-hub", "config.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117},
		Servers: map[string]config.ServerConfig{
			"fixture": {Mode: config.ModeShared, Command: "/bin/true"},
		},
		Clients: map[string]config.ClientManifest{},
	}

	if err := generateClientArtifacts(cfg, "/usr/local/bin/mcp-hub"); err != nil {
		t.Fatalf("generateClientArtifacts: %v", err)
	}

	cursorWrapper := filepath.Join(home, "Applications", "MCP Hub Apps", "Cursor (MCP Hub).app")
	if _, err := os.Stat(cursorWrapper); err != nil {
		t.Fatalf("expected Cursor wrapper to be generated from built-ins: %v", err)
	}

	codexShim := filepath.Join(home, ".mcp-hub", "bin", "codex")
	if _, err := os.Stat(codexShim); err != nil {
		t.Fatalf("expected Codex shim to be generated from built-ins: %v", err)
	}
}
