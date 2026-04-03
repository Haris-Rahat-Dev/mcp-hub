package sidecar_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/sidecar"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")

	state := &sidecar.ClientState{
		ClientID:     "cursor",
		LastSync:     time.Now().Truncate(time.Second),
		ManagedKeys:  []string{"context7", "memory"},
		FileChecksum: "abc123",
		AdapterEntries: map[string]any{
			"context7": map[string]any{"url": "http://127.0.0.1:4117/clients/cursor/mcp/context7"},
		},
	}

	if err := sidecar.Save(cfgPath, state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := sidecar.Load(cfgPath, "cursor")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil state")
	}
	if loaded.ClientID != state.ClientID {
		t.Errorf("ClientID: want %q got %q", state.ClientID, loaded.ClientID)
	}
	if loaded.FileChecksum != state.FileChecksum {
		t.Errorf("FileChecksum: want %q got %q", state.FileChecksum, loaded.FileChecksum)
	}
	if len(loaded.ManagedKeys) != len(state.ManagedKeys) {
		t.Errorf("ManagedKeys len: want %d got %d", len(state.ManagedKeys), len(loaded.ManagedKeys))
	}
}

func TestLoadNonExistentReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")

	state, err := sidecar.Load(cfgPath, "nonexistent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for non-existent client")
	}
}

func TestChecksumDeterministic(t *testing.T) {
	section := map[string]any{
		"server1": map[string]any{"url": "http://example.com"},
		"server2": map[string]any{"url": "http://other.com"},
	}
	sum1, err := sidecar.Checksum(section)
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	sum2, err := sidecar.Checksum(section)
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	if sum1 != sum2 {
		t.Errorf("checksum not deterministic: %q vs %q", sum1, sum2)
	}
	if len(sum1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got %d chars", len(sum1))
	}
}
