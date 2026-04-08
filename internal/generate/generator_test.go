package generate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
)

func TestGenerateStandaloneWritesNamespacedHubURLs(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "cursor-mcp.json")
	cfg := config.Config{
		Path: filepath.Join(tmp, "config.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117},
		Servers: map[string]config.ServerConfig{
			"context7": {Mode: config.ModeShared, Command: "npx"},
		},
		IDEs: map[string]config.IDEConfig{
			"cursor": {
				ConfigPath: target,
				ConfigKey:  "mcpServers",
				URLKey:     "url",
				FileMode:   "standalone",
			},
		},
	}

	_, err := generate.Generate(cfg, "cursor", generate.Options{})
	if err != nil {
		t.Fatalf("generate cursor: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse generated json: %v", err)
	}
	servers := root["mcpServers"].(map[string]any)
	entry := servers["context7"].(map[string]any)
	if got := entry["url"].(string); got != "http://127.0.0.1:4117/mcp/context7" {
		t.Fatalf("unexpected generated url: %s", got)
	}
}

func TestGenerateMergePreservesIDESpecificEntries(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "settings.json")
	existing := []byte(`{"context_servers":{"pencil":{"command":"/bin/pencil","args":["--x"]},"old":{"url":"http://127.0.0.1:4117/mcp/old"}},"theme":"dark"}`)
	if err := os.WriteFile(target, existing, 0o644); err != nil {
		t.Fatalf("seed existing config: %v", err)
	}

	cfg := config.Config{
		Path: filepath.Join(tmp, "config.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117},
		Servers: map[string]config.ServerConfig{
			"context7": {Mode: config.ModeShared, Command: "npx"},
		},
		IDEs: map[string]config.IDEConfig{
			"zed": {
				ConfigPath: target,
				ConfigKey:  "context_servers",
				URLKey:     "url",
				FileMode:   "merge",
			},
		},
	}

	_, err := generate.Generate(cfg, "zed", generate.Options{})
	if err != nil {
		t.Fatalf("generate zed: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read merged file: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse merged json: %v", err)
	}
	if root["theme"].(string) != "dark" {
		t.Fatalf("expected top-level merge preservation")
	}
	section := root["context_servers"].(map[string]any)
	if _, ok := section["pencil"]; !ok {
		t.Fatalf("expected IDE-specific entry to remain")
	}
	if _, ok := section["old"]; ok {
		t.Fatalf("expected old hub-managed entry to be replaced")
	}
	if _, ok := section["context7"]; !ok {
		t.Fatalf("expected generated hub entry")
	}
}
