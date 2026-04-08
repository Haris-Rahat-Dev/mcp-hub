package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type Options struct {
	DryRun bool
	Backup bool
	Merge  bool
}

type Result struct {
	IDE         string
	Path        string
	Written     bool
	Detected    bool
	ServerCount int
	Preserved   int
	BackupPath  string
}

func Generate(cfg config.Config, ideName string, opts Options) (Result, error) {
	defs := ResolveIDEs(cfg)
	def, ok := defs[ideName]
	if !ok {
		return Result{}, fmt.Errorf("unknown IDE %q", ideName)
	}
	return generateForDef(cfg, def, opts)
}

// GenerateForClient generates the adapter config for a specific client manifest.
func GenerateForClient(cfg config.Config, clientID string, manifest config.ClientManifest, opts Options) (Result, error) {
	if manifest.ConfigPath == "" {
		return Result{IDE: clientID, Detected: false}, nil
	}
	return generateForClientManifest(cfg, clientID, manifest, opts)
}

func generateForClientManifest(cfg config.Config, clientID string, manifest config.ClientManifest, opts Options) (Result, error) {
	path := resolvePath(cfg, manifest.ConfigPath)
	mergeMode := opts.Merge || manifest.FileMode == "merge"

	hubServers := buildHubServersForClient(cfg, manifest.ConfigKey, manifest.URLKey, manifest.ExtraFields, manifest.AdapterMode, manifest.BridgeCommand, manifest.BridgeArgs, "")
	preserved := 0
	var out map[string]any

	if mergeMode {
		existing, err := readConfigFile(path, manifest.Format)
		if err != nil && !os.IsNotExist(err) {
			return Result{}, err
		}
		if existing == nil {
			existing = map[string]any{}
		}
		mergedSection, kept := mergeServerSections(existing[manifest.ConfigKey], hubServers)
		preserved = kept
		existing[manifest.ConfigKey] = mergedSection
		out = existing
	} else {
		out = map[string]any{
			manifest.ConfigKey: hubServers,
		}
	}

	format := manifest.Format
	if format == "" {
		format = "json"
	}
	var encoded []byte
	var err error
	switch format {
	case "yaml":
		encoded, err = yaml.Marshal(out)
		if err != nil {
			return Result{}, err
		}
	case "toml":
		encoded, err = toml.Marshal(out)
		if err != nil {
			return Result{}, err
		}
	case "json":
		encoded, err = json.MarshalIndent(out, "", "  ")
		if err != nil {
			return Result{}, err
		}
		encoded = append(encoded, '\n')
	default:
		return Result{}, fmt.Errorf("unsupported client output format %q", format)
	}

	res := Result{
		IDE:         clientID,
		Path:        path,
		Written:     !opts.DryRun,
		Detected:    true,
		ServerCount: len(hubServers),
		Preserved:   preserved,
	}

	if opts.DryRun {
		return res, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, err
	}
	if opts.Backup {
		if backup, err := createBackup(path); err == nil {
			res.BackupPath = backup
		}
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return Result{}, err
	}

	return res, nil
}

func GenerateAllDetected(cfg config.Config, opts Options) ([]Result, error) {
	defs := ResolveIDEs(cfg)
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)

	results := make([]Result, 0, len(names))
	for _, name := range names {
		def := defs[name]
		path := resolvePath(cfg, def.ConfigPath)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				results = append(results, Result{IDE: name, Path: path, Detected: false, Written: false})
				continue
			}
			return nil, err
		}
		res, err := generateForDef(cfg, def, opts)
		if err != nil {
			return nil, err
		}
		res.Detected = true
		results = append(results, res)
	}
	return results, nil
}

func generateForDef(cfg config.Config, def IDEDef, opts Options) (Result, error) {
	path := resolvePath(cfg, def.ConfigPath)
	mergeMode := opts.Merge || def.FileMode == "merge"

	hubServers := buildHubServers(cfg, def)
	preserved := 0
	var out map[string]any

	if mergeMode {
		existing, err := readConfigFile(path, def.Format)
		if err != nil && !os.IsNotExist(err) {
			return Result{}, err
		}
		if existing == nil {
			existing = map[string]any{}
		}

		mergedSection, kept := mergeServerSections(existing[def.ConfigKey], hubServers)
		preserved = kept
		existing[def.ConfigKey] = mergedSection
		out = existing
	} else {
		out = map[string]any{
			def.ConfigKey: hubServers,
		}
	}

	format := def.Format
	if format == "" {
		format = "json"
	}
	var encoded []byte
	var err error
	switch format {
	case "yaml":
		encoded, err = yaml.Marshal(out)
		if err != nil {
			return Result{}, err
		}
	case "toml":
		encoded, err = toml.Marshal(out)
		if err != nil {
			return Result{}, err
		}
	case "json":
		encoded, err = json.MarshalIndent(out, "", "  ")
		if err != nil {
			return Result{}, err
		}
		encoded = append(encoded, '\n')
	default:
		return Result{}, fmt.Errorf("unsupported IDE output format %q", format)
	}

	res := Result{
		IDE:         def.Name,
		Path:        path,
		Written:     !opts.DryRun,
		Detected:    true,
		ServerCount: len(hubServers),
		Preserved:   preserved,
	}

	if opts.DryRun {
		return res, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, err
	}
	if opts.Backup {
		if backup, err := createBackup(path); err == nil {
			res.BackupPath = backup
		}
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return Result{}, err
	}

	return res, nil
}

func buildHubServers(cfg config.Config, def IDEDef) map[string]any {
	return buildHubServersForClient(cfg, def.ConfigKey, def.URLKey, def.Extra, "direct-http", "", nil, "")
}

func readConfigFile(path, format string) (map[string]any, error) {
	switch format {
	case "yaml":
		return readYAMLFile(path)
	case "toml":
		return readTOMLFile(path)
	default:
		return readJSONFile(path)
	}
}

func readYAMLFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func readTOMLFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := toml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// buildHubServersForClient builds the per-server adapter entries for a client.
// For direct-http adapters it emits {urlKey: hubURL, ...extra}.
// For bridge-stdio adapters it emits {command: bridgeCmd, args: [...bridgeArgs with URL substituted], ...extra}.
func buildHubServersForClient(cfg config.Config, configKey, urlKey string, extra map[string]string, adapterMode, bridgeCmd string, bridgeArgs []string, clientID string) map[string]any {
	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make(map[string]any, len(names))
	for _, name := range names {
		serverDef := cfg.Servers[name]

		var hubURL string
		if clientID != "" {
			hubURL = "http://" + cfg.Hub.ListenAddress() + "/clients/" + clientID + "/mcp/" + name
		} else {
			hubURL = "http://" + cfg.Hub.ListenAddress() + "/mcp/" + name
		}

		var entry map[string]any
		switch adapterMode {
		case "bridge-stdio":
			args := make([]string, len(bridgeArgs))
			for i, a := range bridgeArgs {
				if a == "<URL>" {
					args[i] = hubURL
				} else {
					args[i] = a
				}
			}
			cmd := bridgeCmd
			if cmd == "" {
				cmd = "mcp-hub-bridge"
			}
			entry = map[string]any{
				"command": cmd,
				"args":    args,
			}
		default: // "direct-http"
			entry = map[string]any{
				urlKey: hubURL,
			}
		}
		for k, v := range extra {
			entry[k] = v
		}
		// Merge per-client passthrough fields (hub-canonical fields take precedence).
		if clientID != "" && serverDef.Passthrough != nil {
			if ptFields, ok := serverDef.Passthrough[clientID]; ok {
				for k, v := range ptFields {
					if _, exists := entry[k]; !exists {
						entry[k] = v
					}
				}
			}
		}
		out[name] = entry
	}
	return out
}

// urlTransportKeys are the keys that the hub controls and must not be
// preserved from the old entry when replacing a managed server.
var urlTransportKeys = map[string]bool{
	"url":       true,
	"serverUrl": true,
	"command":   true,
	"args":      true,
	"type":      true,
}

func mergeServerSections(existingSection any, hubServers map[string]any) (map[string]any, int) {
	merged := map[string]any{}
	preserved := 0

	existingMap, _ := existingSection.(map[string]any)

	for name, raw := range existingMap {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if isHubManagedEntry(entry) {
			// Preserve app-specific extra fields from the old managed entry
			// so they survive the replacement (fix 0f). Hub-canonical
			// transport keys are overwritten by the new entry below.
			if newEntry, isNew := hubServers[name]; isNew {
				newEntryMap, ok := newEntry.(map[string]any)
				if ok {
					merged[name] = mergePreservingExtras(entry, newEntryMap)
				} else {
					merged[name] = newEntry
				}
			}
			continue
		}
		merged[name] = entry
		preserved++
	}

	for name, entry := range hubServers {
		if _, already := merged[name]; !already {
			merged[name] = entry
		}
	}
	return merged, preserved
}

// mergePreservingExtras returns a copy of newEntry with any keys from oldEntry
// that are not URL/transport keys and not already present in newEntry.
func mergePreservingExtras(oldEntry, newEntry map[string]any) map[string]any {
	out := make(map[string]any, len(newEntry))
	for k, v := range newEntry {
		out[k] = v
	}
	for k, v := range oldEntry {
		if urlTransportKeys[k] {
			continue
		}
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func isHubManagedEntry(entry map[string]any) bool {
	for _, key := range []string{"url", "serverUrl"} {
		raw, ok := entry[key]
		if !ok {
			continue
		}
		url, ok := raw.(string)
		if !ok {
			continue
		}
		if strings.Contains(url, "/mcp/") && strings.Contains(url, "127.0.0.1") {
			return true
		}
	}
	return false
}

var jsoncSanitizer = regexp.MustCompile(`(?s:/\*.*?\*/)|//[^\n]*|"(?:\\.|[^"\\])*"|,(\s*[}\]])`)

func sanitizeJSON(data []byte) []byte {
	return jsoncSanitizer.ReplaceAllFunc(data, func(match []byte) []byte {
		if len(match) > 0 && match[0] == '"' {
			return match
		}
		if len(match) > 0 && match[0] == ',' {
			// keep the whitespace and closing brace/bracket
			return match[1:]
		}
		return []byte("") // strip comment
	})
}

func readJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}

	// Strip comments and trailing commas so standard encoding/json can parse it
	data = sanitizeJSON(data)

	if err := json.Unmarshal(data, &out); err != nil {
		// Include path in error for much better debugging
		return nil, fmt.Errorf("error parsing %s: %w", path, err)
	}
	return out, nil
}

func createBackup(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	backupPath := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102-150405"))
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", err
	}
	return backupPath, nil
}

func resolvePath(cfg config.Config, path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if filepath.IsAbs(path) {
		return path
	}
	base := cfg.BaseDir()
	if base == "" {
		base, _ = os.Getwd()
	}
	return filepath.Clean(filepath.Join(base, path))
}
