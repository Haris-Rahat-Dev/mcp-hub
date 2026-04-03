package generate

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/sidecar"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// SyncResult describes the outcome of syncing one client.
type SyncResult struct {
	ClientID    string
	Path        string
	Written     bool
	DriftWarn   bool
	DriftDetail string
	Preserved   int
	ServerCount int
}

// Sync reconciles a single client's MCP config file with the global hub config.
// It uses sidecar state to detect drift and records ownership after writing.
func Sync(cfg config.Config, clientID string, dryRun bool) (SyncResult, error) {
	clients := ResolveClients(cfg)
	manifest, ok := clients[clientID]
	if !ok {
		return SyncResult{}, fmt.Errorf("unknown client %q", clientID)
	}
	if manifest.ConfigPath == "" {
		return SyncResult{ClientID: clientID}, nil
	}
	return syncClient(cfg, clientID, manifest, dryRun)
}

// SyncAll reconciles all detected clients (those whose config file exists on disk).
func SyncAll(cfg config.Config, dryRun bool) ([]SyncResult, error) {
	clients := ResolveClients(cfg)
	names := make([]string, 0, len(clients))
	for name := range clients {
		names = append(names, name)
	}
	sort.Strings(names)

	var results []SyncResult
	for _, name := range names {
		manifest := clients[name]
		if manifest.ConfigPath == "" {
			continue
		}
		path := resolvePath(cfg, manifest.ConfigPath)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				results = append(results, SyncResult{ClientID: name, Path: path})
				continue
			}
			return nil, err
		}
		res, err := syncClient(cfg, name, manifest, dryRun)
		if err != nil {
			return nil, fmt.Errorf("sync %s: %w", name, err)
		}
		results = append(results, res)
	}
	return results, nil
}

func syncClient(cfg config.Config, clientID string, manifest config.ClientManifest, dryRun bool) (SyncResult, error) {
	path := resolvePath(cfg, manifest.ConfigPath)

	// Build target adapter entries using client-scoped URLs.
	hubServers := buildHubServersForClient(
		cfg,
		manifest.ConfigKey,
		manifest.URLKey,
		manifest.ExtraFields,
		manifest.AdapterMode,
		manifest.BridgeCommand,
		manifest.BridgeArgs,
		clientID,
	)

	// Load existing client config file.
	var existingFile map[string]any
	switch manifest.Format {
	case "yaml":
		existingFile = readYAMLFileOrEmpty(path)
	case "toml":
		existingFile = readTOMLFileOrEmpty(path)
	default:
		existing, err := readJSONFile(path)
		if err != nil && !os.IsNotExist(err) {
			return SyncResult{}, fmt.Errorf("read %s: %w", path, err)
		}
		if existing == nil {
			existing = map[string]any{}
		}
		existingFile = existing
	}

	// Extract the current managed section for drift detection.
	currentSection, _ := existingFile[manifest.ConfigKey].(map[string]any)

	// Load sidecar state.
	state, err := sidecar.Load(cfg.Path, clientID)
	if err != nil {
		return SyncResult{}, err
	}

	res := SyncResult{
		ClientID:    clientID,
		Path:        path,
		ServerCount: len(hubServers),
	}

	// Drift detection: compare current managed section checksum against recorded.
	if state != nil && currentSection != nil {
		currentChecksum, csErr := sidecar.Checksum(currentSection)
		if csErr == nil && currentChecksum != state.FileChecksum {
			res.DriftWarn = true
			res.DriftDetail = fmt.Sprintf(
				"managed section checksum mismatch (expected %s, got %s); overwriting",
				state.FileChecksum[:8], currentChecksum[:8],
			)
			log.Printf("[sync] drift detected for client %q: %s", clientID, res.DriftDetail)
		}
	}

	// Determine managed keys: use sidecar if available, else fall back to URL heuristic.
	var managedKeys map[string]bool
	if state != nil && len(state.ManagedKeys) > 0 {
		managedKeys = make(map[string]bool, len(state.ManagedKeys))
		for _, k := range state.ManagedKeys {
			managedKeys[k] = true
		}
	}

	// Merge: replace managed section, preserve unmanaged entries.
	var mergedSection map[string]any
	var preserved int
	if manifest.FileMode == "merge" {
		mergedSection, preserved = mergeServerSectionsWithKeys(currentSection, hubServers, managedKeys)
	} else {
		mergedSection = hubServers
	}
	res.Preserved = preserved

	existingFile[manifest.ConfigKey] = mergedSection

	if dryRun {
		return res, nil
	}

	// Encode and write atomically.
	var encoded []byte
	switch manifest.Format {
	case "yaml":
		encoded, err = yaml.Marshal(existingFile)
		if err != nil {
			return SyncResult{}, err
		}
	case "toml":
		encoded, err = toml.Marshal(existingFile)
		if err != nil {
			return SyncResult{}, err
		}
	default:
		encoded, err = json.MarshalIndent(existingFile, "", "  ")
		if err != nil {
			return SyncResult{}, err
		}
		encoded = append(encoded, '\n')
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return SyncResult{}, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return SyncResult{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return SyncResult{}, err
	}
	res.Written = true

	// Record new sidecar state.
	newManagedKeys := make([]string, 0, len(hubServers))
	for k := range hubServers {
		newManagedKeys = append(newManagedKeys, k)
	}
	sort.Strings(newManagedKeys)

	newChecksum, _ := sidecar.Checksum(mergedSection)
	adapterEntries := make(map[string]any, len(hubServers))
	for k, v := range hubServers {
		adapterEntries[k] = v
	}
	newState := &sidecar.ClientState{
		ClientID:       clientID,
		LastSync:       time.Now(),
		ManagedKeys:    newManagedKeys,
		FileChecksum:   newChecksum,
		AdapterEntries: adapterEntries,
	}
	if err := sidecar.Save(cfg.Path, newState); err != nil {
		return res, fmt.Errorf("save sidecar state: %w", err)
	}

	return res, nil
}

// mergeServerSectionsWithKeys is like mergeServerSections but uses an explicit
// set of managed keys (from sidecar) instead of the URL heuristic. Falls back
// to the URL heuristic when managedKeys is nil (first sync).
func mergeServerSectionsWithKeys(existingSection map[string]any, hubServers map[string]any, managedKeys map[string]bool) (map[string]any, int) {
	if managedKeys == nil {
		// First sync: use URL heuristic for one-time ownership migration.
		result, preserved := mergeServerSections(existingSection, hubServers)
		return result, preserved
	}

	merged := map[string]any{}
	preserved := 0

	for name, raw := range existingSection {
		if managedKeys[name] {
			// Managed entry: replace with hub version (preserving extras).
			if newEntry, isNew := hubServers[name]; isNew {
				newEntryMap, ok := newEntry.(map[string]any)
				oldEntryMap, oldOk := raw.(map[string]any)
				if ok && oldOk {
					merged[name] = mergePreservingExtras(oldEntryMap, newEntryMap)
				} else {
					merged[name] = newEntry
				}
			}
			continue
		}
		merged[name] = raw
		preserved++
	}

	for name, entry := range hubServers {
		if _, already := merged[name]; !already {
			merged[name] = entry
		}
	}
	return merged, preserved
}

func readYAMLFileOrEmpty(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func readTOMLFileOrEmpty(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := toml.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	if out == nil {
		return map[string]any{}
	}
	return out
}
