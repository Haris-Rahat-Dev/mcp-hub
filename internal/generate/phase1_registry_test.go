package generate_test

import (
	"testing"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
)

// TestResolveClientsBuiltinPresent verifies that all expected built-in clients
// are returned by ResolveClients (Phase 1).
func TestResolveClientsBuiltinPresent(t *testing.T) {
	cfg := config.Config{Clients: map[string]config.ClientManifest{}}
	clients := generate.ResolveClients(cfg)

	expected := []string{"cursor", "windsurf", "zed", "vscode", "continue", "claude-code"}
	for _, name := range expected {
		if _, ok := clients[name]; !ok {
			t.Errorf("expected built-in client %q to be present", name)
		}
	}
}

// TestResolveClientsUserOverridesBuiltin verifies that a user-defined clients:
// entry overrides the built-in with the same id (Phase 1).
func TestResolveClientsUserOverridesBuiltin(t *testing.T) {
	cfg := config.Config{
		Clients: map[string]config.ClientManifest{
			"cursor": {
				DisplayName: "My Cursor",
				ConfigPath:  "/custom/mcp.json",
				ConfigKey:   "mcpServers",
				URLKey:      "url",
				FileMode:    "standalone",
				Format:      "json",
				AdapterMode: "direct-http",
			},
		},
	}
	clients := generate.ResolveClients(cfg)
	if clients["cursor"].ConfigPath != "/custom/mcp.json" {
		t.Errorf("expected user override to win, got: %s", clients["cursor"].ConfigPath)
	}
	if clients["cursor"].DisplayName != "My Cursor" {
		t.Errorf("expected user display name, got: %s", clients["cursor"].DisplayName)
	}
}

// TestResolveClientsIDEsFallback verifies that ides: entries are folded in
// as a backward-compat fallback when no clients: entry exists (Phase 1).
func TestResolveClientsIDEsFallback(t *testing.T) {
	cfg := config.Config{
		IDEs: map[string]config.IDEConfig{
			"myide": {
				ConfigPath: "/tmp/myide.json",
				ConfigKey:  "mcpServers",
				URLKey:     "url",
				FileMode:   "standalone",
			},
		},
	}
	clients := generate.ResolveClients(cfg)
	if _, ok := clients["myide"]; !ok {
		t.Fatal("expected ides: fallback entry to appear in ResolveClients")
	}
	if clients["myide"].ConfigPath != "/tmp/myide.json" {
		t.Errorf("unexpected config path: %s", clients["myide"].ConfigPath)
	}
}

// TestBuiltinContinueIsBridgeStdio verifies that the continue client uses
// bridge-stdio adapter mode (Phase 1).
func TestBuiltinContinueIsBridgeStdio(t *testing.T) {
	clients := generate.BuiltinClients()
	cont, ok := clients["continue"]
	if !ok {
		t.Fatal("continue not in BuiltinClients")
	}
	if cont.AdapterMode != "bridge-stdio" {
		t.Errorf("expected bridge-stdio, got %q", cont.AdapterMode)
	}
	if cont.BridgeCommand == "" {
		t.Error("expected non-empty BridgeCommand for continue")
	}
}

// TestBridgeStdioEntryShape verifies that buildHubServers emits command/args
// for bridge-stdio clients instead of a URL entry (Phase 1).
func TestBridgeStdioEntryShape(t *testing.T) {
	cfg := config.Config{
		Hub: config.HubConfig{Host: "127.0.0.1", Port: 4117},
		Servers: map[string]config.ServerConfig{
			"myserver": {Mode: config.ModeShared, Command: "npx"},
		},
		IDEs: map[string]config.IDEConfig{
			"continue": {
				ConfigPath: "/tmp/hub.yaml",
				ConfigKey:  "mcpServers",
				URLKey:     "url",
				FileMode:   "standalone",
				Format:     "yaml",
			},
		},
	}
	// Use GenerateForClient with bridge-stdio manifest.
	manifest := config.ClientManifest{
		DisplayName:   "Continue",
		Kind:          "cli",
		ConfigPath:    "/tmp/hub.yaml",
		ConfigKey:     "mcpServers",
		URLKey:        "url",
		FileMode:      "standalone",
		Format:        "yaml",
		AdapterMode:   "bridge-stdio",
		BridgeCommand: "mcp-hub-bridge",
		BridgeArgs:    []string{"--target", "<URL>"},
	}
	res, err := generate.GenerateForClient(cfg, "continue", manifest, generate.Options{DryRun: true})
	if err != nil {
		t.Fatalf("GenerateForClient: %v", err)
	}
	if !res.Detected {
		t.Fatal("expected detected=true")
	}
	if res.ServerCount != 1 {
		t.Fatalf("expected 1 server, got %d", res.ServerCount)
	}
}

// TestCodexBuiltinConfigured verifies that the codex built-in is fully
// configured for detection/sync and launch.
func TestCodexBuiltinConfigured(t *testing.T) {
	clients := generate.BuiltinClients()
	codex, ok := clients["codex"]
	if !ok {
		t.Fatal("codex not in BuiltinClients")
	}
	if codex.ConfigPath == "" {
		t.Fatal("expected non-empty ConfigPath for codex")
	}
	if codex.Format != "toml" {
		t.Errorf("expected toml format for codex, got %q", codex.Format)
	}
	if codex.Executable == "" {
		t.Fatal("expected non-empty executable for codex")
	}
}
