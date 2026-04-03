package generate

import "github.com/harisbinrahat/mcp-hub/internal/config"

// IDEDef is the legacy resolved client descriptor. New code should use
// config.ClientManifest via ResolveClients instead.
type IDEDef struct {
	Name       string
	ConfigPath string
	ConfigKey  string
	URLKey     string
	FileMode   string
	Format     string
	Extra      map[string]string
}

// BuiltinClients returns the canonical set of built-in client manifests.
// Add new app support here; user overrides come from cfg.Clients in ResolveClients.
func BuiltinClients() map[string]config.ClientManifest {
	ss := func(m map[string]string) map[string]string { return m }
	return map[string]config.ClientManifest{
		"claude-code": {
			DisplayName: "Claude Code", Kind: "cli",
			ConfigPath: "~/.claude.json", ConfigKey: "mcpServers", URLKey: "url",
			FileMode: "merge", Format: "json", AdapterMode: "direct-http",
			ExtraFields: ss(map[string]string{"type": "sse"}),
		},
		"cursor": {
			DisplayName: "Cursor", Kind: "gui",
			BundleID:   "com.todesktop.230313mzl4w4u92",
			ConfigPath: "~/.cursor/mcp.json", ConfigKey: "mcpServers", URLKey: "url",
			FileMode: "standalone", Format: "json", AdapterMode: "direct-http",
		},
		"windsurf": {
			DisplayName: "Windsurf", Kind: "gui",
			BundleID:   "com.exafunction.windsurf",
			ConfigPath: "~/.codeium/windsurf/mcp_config.json", ConfigKey: "mcpServers", URLKey: "serverUrl",
			FileMode: "standalone", Format: "json", AdapterMode: "direct-http",
		},
		"kiro": {
			DisplayName: "Kiro", Kind: "gui",
			ConfigPath: "~/.kiro/settings/mcp.json", ConfigKey: "mcpServers", URLKey: "url",
			FileMode: "standalone", Format: "json", AdapterMode: "direct-http",
		},
		"kimi": {
			DisplayName: "Kimi", Kind: "gui",
			ConfigPath: "~/.kimi/mcp.json", ConfigKey: "mcpServers", URLKey: "url",
			FileMode: "standalone", Format: "json", AdapterMode: "direct-http",
		},
		"lmstudio": {
			DisplayName: "LM Studio", Kind: "gui",
			BundleID:   "ai.lmstudio.app",
			ConfigPath: "~/.lmstudio/mcp.json", ConfigKey: "mcpServers", URLKey: "url",
			FileMode: "standalone", Format: "json", AdapterMode: "direct-http",
		},
		"void": {
			DisplayName: "Void", Kind: "gui",
			ConfigPath: "~/.void-editor/mcp.json", ConfigKey: "mcpServers", URLKey: "url",
			FileMode: "standalone", Format: "json", AdapterMode: "direct-http",
		},
		"gemini": {
			DisplayName: "Gemini", Kind: "gui",
			ConfigPath: "~/.gemini/antigravity/mcp_config.json", ConfigKey: "mcpServers", URLKey: "serverUrl",
			FileMode: "standalone", Format: "json", AdapterMode: "direct-http",
		},
		"zed": {
			DisplayName: "Zed", Kind: "gui",
			BundleID:   "dev.zed.Zed",
			ConfigPath: "~/.config/zed/settings.json", ConfigKey: "context_servers", URLKey: "url",
			FileMode: "merge", Format: "json", AdapterMode: "direct-http",
		},
		"vscode": {
			DisplayName: "VS Code", Kind: "gui",
			BundleID:   "com.microsoft.VSCode",
			ConfigPath: ".vscode/mcp.json", ConfigKey: "servers", URLKey: "url",
			FileMode: "standalone", Format: "json", AdapterMode: "direct-http",
			ExtraFields: ss(map[string]string{"type": "http"}),
		},
		"trae": {
			DisplayName: "Trae", Kind: "gui",
			ConfigPath: ".trae/mcp.json", ConfigKey: "mcpServers", URLKey: "url",
			FileMode: "standalone", Format: "json", AdapterMode: "direct-http",
		},
		"continue": {
			DisplayName: "Continue", Kind: "cli",
			ConfigPath: ".continue/mcpServers/hub.yaml", ConfigKey: "mcpServers", URLKey: "url",
			FileMode: "standalone", Format: "yaml", AdapterMode: "bridge-stdio",
			BridgeCommand: "mcp-hub-bridge",
			BridgeArgs:    []string{"--target", "<URL>"},
		},
		"codex": {
			DisplayName: "Codex", Kind: "cli",
			Executable: "codex",
			ConfigPath: "~/.codex/config.toml", ConfigKey: "mcp_servers", URLKey: "url",
			FileMode: "merge", Format: "toml", AdapterMode: "bridge-stdio",
			BridgeCommand: "mcp-hub-bridge",
			BridgeArgs:    []string{"--target", "<URL>"},
		},
	}
}

// ResolveClients merges built-in manifests with user-defined overrides.
// Merge order: BuiltinClients → cfg.IDEs fallback → cfg.Clients (same id wins).
// Defaults are applied to any fields left empty.
func ResolveClients(cfg config.Config) map[string]config.ClientManifest {
	out := BuiltinClients()

	// Fold in cfg.IDEs as a backward-compat fallback. An IDEs entry overrides
	// the builtin only when it has a non-empty ConfigPath (i.e. user-specified).
	for name, ide := range cfg.IDEs {
		if ide.ConfigPath == "" {
			continue
		}
		m := config.ClientManifest{
			DisplayName: name,
			Kind:        "gui",
			ConfigPath:  ide.ConfigPath,
			ConfigKey:   ide.ConfigKey,
			URLKey:      ide.URLKey,
			FileMode:    ide.FileMode,
			Format:      ide.Format,
			ExtraFields: ide.ExtraFields,
			AdapterMode: "direct-http",
		}
		applyClientDefaults(&m)
		out[name] = m
	}

	// cfg.Clients wins over both builtins and IDEs fallback.
	for name, m := range cfg.Clients {
		if m.ConfigPath == "" {
			continue
		}
		applyClientDefaults(&m)
		out[name] = m
	}

	return out
}

func applyClientDefaults(m *config.ClientManifest) {
	if m.Kind == "" {
		m.Kind = "gui"
	}
	if m.ConfigKey == "" {
		m.ConfigKey = "mcpServers"
	}
	if m.URLKey == "" {
		m.URLKey = "url"
	}
	if m.FileMode == "" {
		m.FileMode = "standalone"
	}
	if m.Format == "" {
		m.Format = "json"
	}
	if m.AdapterMode == "" {
		m.AdapterMode = "direct-http"
	}
	if m.AdapterMode == "bridge-stdio" && m.BridgeCommand == "" {
		m.BridgeCommand = "mcp-hub-bridge"
	}
	if m.AdapterMode == "bridge-stdio" && len(m.BridgeArgs) == 0 {
		m.BridgeArgs = []string{"--target", "<URL>"}
	}
}

// BuiltinIDEs returns the legacy IDEDef map for backward compatibility.
// Deprecated: use BuiltinClients and ResolveClients instead.
func BuiltinIDEs() map[string]IDEDef {
	clients := BuiltinClients()
	out := make(map[string]IDEDef, len(clients))
	for name, m := range clients {
		out[name] = clientToIDEDef(name, m)
	}
	return out
}

// ResolveIDEs returns the legacy IDEDef map for backward compatibility.
// Deprecated: use ResolveClients instead.
func ResolveIDEs(cfg config.Config) map[string]IDEDef {
	clients := ResolveClients(cfg)
	out := make(map[string]IDEDef, len(clients))
	for name, m := range clients {
		out[name] = clientToIDEDef(name, m)
	}
	return out
}

func clientToIDEDef(name string, m config.ClientManifest) IDEDef {
	return IDEDef{
		Name:       name,
		ConfigPath: m.ConfigPath,
		ConfigKey:  m.ConfigKey,
		URLKey:     m.URLKey,
		FileMode:   m.FileMode,
		Format:     m.Format,
		Extra:      m.ExtraFields,
	}
}
