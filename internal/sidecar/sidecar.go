package sidecar

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ClientState records the last-synced state for a managed client. It is stored
// as a JSON file under <cfg-dir>/state/<client-id>.json and used to detect
// drift (when the app or user modifies managed MCP entries between syncs).
type ClientState struct {
	ClientID       string         `json:"client_id"`
	LastSync       time.Time      `json:"last_sync"`
	ManagedKeys    []string       `json:"managed_keys"`
	FileChecksum   string         `json:"file_checksum"` // SHA-256 of managed section JSON bytes
	AdapterEntries map[string]any `json:"adapter_entries"`
}

// StateDir returns the directory where per-client state files are stored.
func StateDir(cfgPath string) string {
	return filepath.Join(filepath.Dir(cfgPath), "state")
}

func statePath(cfgPath, clientID string) string {
	return filepath.Join(StateDir(cfgPath), clientID+".json")
}

// Load reads the state file for the given client. Returns nil (not an error)
// if no state file exists yet (first sync).
func Load(cfgPath, clientID string) (*ClientState, error) {
	path := statePath(cfgPath, clientID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("sidecar: read %s: %w", path, err)
	}
	var s ClientState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("sidecar: parse %s: %w", path, err)
	}
	return &s, nil
}

// Save writes the state file atomically (write to .tmp then rename).
func Save(cfgPath string, s *ClientState) error {
	dir := StateDir(cfgPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("sidecar: mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("sidecar: marshal: %w", err)
	}
	data = append(data, '\n')

	dest := statePath(cfgPath, s.ClientID)
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("sidecar: write tmp: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("sidecar: rename: %w", err)
	}
	return nil
}

// Checksum returns the SHA-256 hex digest of the canonical JSON representation
// of the managed section (a map[string]any). Used for drift detection.
func Checksum(section map[string]any) (string, error) {
	b, err := json.Marshal(section)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
