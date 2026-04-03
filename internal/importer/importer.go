package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/harisbinrahat/mcp-hub/internal/config"
)

// ImportMCPJSON merges servers from a Cursor-style mcp.json blob into the hub config file.
// clientID is optional; when set, unmappable fields are stored under ServerConfig.Passthrough.
func ImportMCPJSON(configPath string, srcBytes []byte, clientID string) (imported int, err error) {
	var src map[string]any
	if err := json.Unmarshal(srcBytes, &src); err != nil {
		return 0, err
	}
	rawServers, ok := src["mcpServers"].(map[string]any)
	if !ok {
		return 0, fmt.Errorf("source config has no mcpServers object")
	}

	cfg, err := config.LoadForMutation(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			path, rerr := config.ResolveConfigPath(configPath)
			if rerr != nil {
				return 0, rerr
			}
			cfg = config.Config{
				Path: path,
				Hub: config.HubConfig{
					Host:     "127.0.0.1",
					Port:     4117,
					LogLevel: "info",
					LogFile:  filepath.Join(filepath.Dir(path), "hub.log"),
				},
				Servers: map[string]config.ServerConfig{},
				Clients: map[string]config.ClientManifest{},
				IDEs:    map[string]config.IDEConfig{},
			}
		} else {
			return 0, err
		}
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]config.ServerConfig{}
	}

	isolatedNames := []string{"puppeteer", "playwright", "sequential-thinking", "git"}
	imported = 0
	for name, raw := range rawServers {
		obj, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		envMap := parseStringMap(obj["env"])
		headersMap := parseStringMap(obj["headers"])
		lazyVal := parseBoolPtr(obj["disabled"], obj["lazy"])
		maxInst := parseInt(obj["maxInstances"], obj["max_instances"])

		if u, ok := obj["url"].(string); ok && strings.TrimSpace(u) != "" {
			if strings.Contains(u, "127.0.0.1") || strings.Contains(u, "localhost") {
				continue
			}
			sc := config.ServerConfig{
				URL:     u,
				Mode:    config.ModePassthrough,
				Headers: headersMap,
			}
			if lazyVal != nil {
				sc.Lazy = lazyVal
			}
			if maxInst > 0 {
				sc.MaxInstances = maxInst
			}
			sc = applyPassthrough(sc, obj, clientID, []string{"url", "headers", "disabled", "lazy", "maxInstances", "max_instances"})
			cfg.Servers[name] = sc
			imported++
			continue
		}
		if u, ok := obj["serverUrl"].(string); ok && strings.TrimSpace(u) != "" {
			if strings.Contains(u, "127.0.0.1") || strings.Contains(u, "localhost") {
				continue
			}
			sc := config.ServerConfig{
				URL:     u,
				Mode:    config.ModePassthrough,
				Headers: headersMap,
			}
			if lazyVal != nil {
				sc.Lazy = lazyVal
			}
			if maxInst > 0 {
				sc.MaxInstances = maxInst
			}
			sc = applyPassthrough(sc, obj, clientID, []string{"serverUrl", "headers", "disabled", "lazy", "maxInstances", "max_instances"})
			cfg.Servers[name] = sc
			imported++
			continue
		}
		cmd, _ := obj["command"].(string)
		var parsedArgs []string
		if argsAny, ok := obj["args"].([]any); ok {
			for _, a := range argsAny {
				if s, ok := a.(string); ok {
					parsedArgs = append(parsedArgs, s)
				}
			}
		}
		if cmd == "" {
			continue
		}
		mode := config.ModeShared
		if slices.Contains(isolatedNames, name) {
			mode = config.ModeIsolated
		}
		sc := config.ServerConfig{
			Command: cmd,
			Args:    parsedArgs,
			Mode:    mode,
			Env:     envMap,
			Headers: headersMap,
		}
		if lazyVal != nil {
			sc.Lazy = lazyVal
		}
		if maxInst > 0 {
			sc.MaxInstances = maxInst
		}
		sc = applyPassthrough(sc, obj, clientID, []string{"command", "args", "env", "headers", "disabled", "lazy", "maxInstances", "max_instances"})
		cfg.Servers[name] = sc
		imported++
	}

	if err := config.Save(cfg); err != nil {
		return imported, err
	}
	return imported, nil
}

func applyPassthrough(sc config.ServerConfig, obj map[string]any, clientID string, knownKeys []string) config.ServerConfig {
	if clientID == "" {
		return sc
	}
	known := make(map[string]bool, len(knownKeys))
	for _, k := range knownKeys {
		known[k] = true
	}
	extras := map[string]any{}
	for k, v := range obj {
		if !known[k] {
			extras[k] = v
		}
	}
	if len(extras) > 0 {
		if sc.Passthrough == nil {
			sc.Passthrough = map[string]map[string]any{}
		}
		sc.Passthrough[clientID] = extras
	}
	return sc
}

func parseStringMap(v any) map[string]string {
	raw, ok := v.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, val := range raw {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseBoolPtr(candidates ...any) *bool {
	for _, v := range candidates {
		switch val := v.(type) {
		case bool:
			b := val
			return &b
		case float64:
			b := val != 0
			return &b
		}
	}
	return nil
}

func parseInt(candidates ...any) int {
	for _, v := range candidates {
		switch val := v.(type) {
		case float64:
			if i := int(val); i > 0 {
				return i
			}
		case int:
			if val > 0 {
				return val
			}
		}
	}
	return 0
}
