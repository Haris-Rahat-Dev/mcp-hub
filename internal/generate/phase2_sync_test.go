package generate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
	"github.com/harisbinrahat/mcp-hub/internal/sidecar"
	"github.com/pelletier/go-toml/v2"
)

func makeSyncCfg(t *testing.T, clientConfigPath string) config.Config {
	t.Helper()
	tmp := t.TempDir()
	return config.Config{
		Path: filepath.Join(tmp, "config.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117},
		Servers: map[string]config.ServerConfig{
			"myserver": {Mode: config.ModeShared, Command: "npx"},
		},
		Clients: map[string]config.ClientManifest{
			"testclient": {
				DisplayName: "Test",
				Kind:        "gui",
				ConfigPath:  clientConfigPath,
				ConfigKey:   "mcpServers",
				URLKey:      "url",
				FileMode:    "standalone",
				Format:      "json",
				AdapterMode: "direct-http",
			},
		},
	}
}

// TestSyncWritesClientScopedURL verifies that Sync emits /clients/<id>/mcp/<server>
// URLs in the generated config (Phase 2 + Phase 4d).
func TestSyncWritesClientScopedURL(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "mcp.json")
	cfg := makeSyncCfg(t, target)

	res, err := generate.Sync(cfg, "testclient", false)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !res.Written {
		t.Fatal("expected Written=true")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	servers := root["mcpServers"].(map[string]any)
	entry := servers["myserver"].(map[string]any)
	gotURL, _ := entry["url"].(string)
	wantURL := "http://127.0.0.1:4117/clients/testclient/mcp/myserver"
	if gotURL != wantURL {
		t.Errorf("expected client-scoped URL %q, got %q", wantURL, gotURL)
	}
}

// TestSyncSidecarSaveLoad verifies that sidecar state is written after Sync
// and can be loaded back (Phase 2).
func TestSyncSidecarSaveLoad(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "mcp.json")
	cfg := makeSyncCfg(t, target)

	if _, err := generate.Sync(cfg, "testclient", false); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	state, err := sidecar.Load(cfg.Path, "testclient")
	if err != nil {
		t.Fatalf("sidecar.Load: %v", err)
	}
	if state == nil {
		t.Fatal("expected sidecar state to be written")
	}
	if state.ClientID != "testclient" {
		t.Errorf("unexpected client id: %s", state.ClientID)
	}
	if len(state.ManagedKeys) == 0 {
		t.Error("expected non-empty ManagedKeys")
	}
	if state.FileChecksum == "" {
		t.Error("expected non-empty FileChecksum")
	}
}

// TestSyncDriftDetection verifies that modifying a managed entry between syncs
// triggers DriftWarn on the next sync (Phase 2).
func TestSyncDriftDetection(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "mcp.json")
	cfg := makeSyncCfg(t, target)

	// First sync — establishes sidecar state.
	if _, err := generate.Sync(cfg, "testclient", false); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	// Simulate drift: overwrite the managed section with different content.
	drifted := []byte(`{"mcpServers":{"myserver":{"url":"http://tampered:9999/mcp/myserver"}}}` + "\n")
	if err := os.WriteFile(target, drifted, 0o644); err != nil {
		t.Fatalf("write drifted config: %v", err)
	}

	// Second sync — should detect drift.
	res, err := generate.Sync(cfg, "testclient", false)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if !res.DriftWarn {
		t.Error("expected DriftWarn=true after manual edit")
	}
}

// TestSyncMergePreservesUnmanaged verifies that merge-mode sync preserves
// entries not managed by the hub (Phase 2).
func TestSyncMergePreservesUnmanaged(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "settings.json")
	existing := []byte(`{"mcpServers":{"userentry":{"url":"http://user.example.com/mcp"}},"theme":"dark"}` + "\n")
	if err := os.WriteFile(target, existing, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg := config.Config{
		Path: filepath.Join(tmp, "config.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117},
		Servers: map[string]config.ServerConfig{
			"hubserver": {Mode: config.ModeShared, Command: "npx"},
		},
		Clients: map[string]config.ClientManifest{
			"mergeclient": {
				DisplayName: "Merge",
				Kind:        "gui",
				ConfigPath:  target,
				ConfigKey:   "mcpServers",
				URLKey:      "url",
				FileMode:    "merge",
				Format:      "json",
				AdapterMode: "direct-http",
			},
		},
	}

	if _, err := generate.Sync(cfg, "mergeclient", false); err != nil {
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
	if root["theme"].(string) != "dark" {
		t.Error("expected top-level theme to be preserved")
	}
	servers := root["mcpServers"].(map[string]any)
	if _, ok := servers["userentry"]; !ok {
		t.Error("expected unmanaged userentry to be preserved")
	}
	if _, ok := servers["hubserver"]; !ok {
		t.Error("expected hub-managed hubserver to be present")
	}
}

// TestSyncDryRunDoesNotWrite verifies that --dry-run does not write files (Phase 2).
func TestSyncDryRunDoesNotWrite(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "mcp.json")
	cfg := makeSyncCfg(t, target)

	res, err := generate.Sync(cfg, "testclient", true)
	if err != nil {
		t.Fatalf("Sync dry-run: %v", err)
	}
	if res.Written {
		t.Error("expected Written=false for dry-run")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file not to be written in dry-run mode")
	}
}

func TestSyncTOMLMergeWritesBridgeEntries(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "config.toml")
	seed := `theme = "dark"

[mcp_servers.existing]
command = "node"
args = ["legacy.js"]
`
	if err := os.WriteFile(target, []byte(seed), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg := config.Config{
		Path: filepath.Join(tmp, "hub.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117},
		Servers: map[string]config.ServerConfig{
			"hubserver": {Mode: config.ModeShared, Command: "npx"},
		},
		Clients: map[string]config.ClientManifest{
			"codex": {
				DisplayName:   "Codex",
				Kind:          "cli",
				ConfigPath:    target,
				ConfigKey:     "mcp_servers",
				FileMode:      "merge",
				Format:        "toml",
				AdapterMode:   "bridge-stdio",
				BridgeCommand: "mcp-hub-bridge",
				BridgeArgs:    []string{"--target", "<URL>"},
			},
		},
	}

	if _, err := generate.Sync(cfg, "codex", false); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse toml: %v", err)
	}
	if root["theme"].(string) != "dark" {
		t.Fatal("expected top-level theme to be preserved")
	}
	servers := root["mcp_servers"].(map[string]any)
	entry := servers["hubserver"].(map[string]any)
	if entry["command"] != "mcp-hub-bridge" {
		t.Fatalf("unexpected command: %#v", entry["command"])
	}
	args := entry["args"].([]any)
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	wantURL := "http://127.0.0.1:4117/clients/codex/mcp/hubserver"
	if args[1] != wantURL {
		t.Fatalf("expected bridge URL %q, got %#v", wantURL, args[1])
	}
}
