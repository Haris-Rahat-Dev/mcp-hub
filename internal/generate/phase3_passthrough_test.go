package generate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
)

// TestPassthroughFieldsMergedIntoAdapterEntry verifies that per-client
// Passthrough fields are included in the generated adapter entry (Phase 3).
func TestPassthroughFieldsMergedIntoAdapterEntry(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "mcp.json")

	cfg := config.Config{
		Path: filepath.Join(tmp, "config.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117},
		Servers: map[string]config.ServerConfig{
			"myserver": {
				Mode:    config.ModeShared,
				Command: "npx",
				Passthrough: map[string]map[string]any{
					"testclient": {"extraKey": "extraValue", "anotherKey": 42},
				},
			},
		},
		Clients: map[string]config.ClientManifest{
			"testclient": {
				DisplayName: "Test",
				Kind:        "gui",
				ConfigPath:  target,
				ConfigKey:   "mcpServers",
				URLKey:      "url",
				FileMode:    "standalone",
				Format:      "json",
				AdapterMode: "direct-http",
			},
		},
	}

	if _, err := generate.Sync(cfg, "testclient", false); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse: %v", err)
	}
	servers := root["mcpServers"].(map[string]any)
	entry := servers["myserver"].(map[string]any)

	if entry["extraKey"] != "extraValue" {
		t.Errorf("expected extraKey=extraValue in entry, got: %v", entry)
	}
	// Hub-canonical url must not be overwritten by passthrough.
	if entry["url"] == "" {
		t.Error("expected url to be present")
	}
}

// TestPassthroughHubFieldsNotOverwritten verifies that hub-canonical fields
// (url, command, args) cannot be overridden by Passthrough (Phase 3).
func TestPassthroughHubFieldsNotOverwritten(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "mcp.json")

	cfg := config.Config{
		Path: filepath.Join(tmp, "config.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117},
		Servers: map[string]config.ServerConfig{
			"myserver": {
				Mode:    config.ModeShared,
				Command: "npx",
				Passthrough: map[string]map[string]any{
					"testclient": {"url": "http://attacker.example.com/mcp"},
				},
			},
		},
		Clients: map[string]config.ClientManifest{
			"testclient": {
				DisplayName: "Test",
				Kind:        "gui",
				ConfigPath:  target,
				ConfigKey:   "mcpServers",
				URLKey:      "url",
				FileMode:    "standalone",
				Format:      "json",
				AdapterMode: "direct-http",
			},
		},
	}

	if _, err := generate.Sync(cfg, "testclient", false); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	data, _ := os.ReadFile(target)
	var root map[string]any
	_ = json.Unmarshal(data, &root)
	servers := root["mcpServers"].(map[string]any)
	entry := servers["myserver"].(map[string]any)

	gotURL, _ := entry["url"].(string)
	if gotURL == "http://attacker.example.com/mcp" {
		t.Error("passthrough must not override hub-canonical url field")
	}
}
