package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestImportPreservesEnvHeadersLazyMaxInstances verifies that runImport
// correctly preserves env, headers, lazy, and max_instances fields that were
// previously silently dropped (fix 0e).
func TestImportPreservesEnvHeadersLazyMaxInstances(t *testing.T) {
	tmp := t.TempDir()
	srcPath := filepath.Join(tmp, "mcp.json")
	cfgPath := filepath.Join(tmp, "config.yaml")

	// Seed a config.yaml with at least one server (required by config.validate).
	seedCfg := `hub:
  host: 127.0.0.1
  port: 4117
servers:
  placeholder:
    command: echo
    mode: shared
`
	if err := os.WriteFile(cfgPath, []byte(seedCfg), 0o644); err != nil {
		t.Fatalf("write seed config: %v", err)
	}

	src := map[string]any{
		"mcpServers": map[string]any{
			"myserver": map[string]any{
				"command":      "node",
				"args":         []any{"/path/to/server.js"},
				"env":          map[string]any{"API_KEY": "secret123", "DEBUG": "true"},
				"headers":      map[string]any{"X-Custom": "value"},
				"lazy":         true,
				"maxInstances": float64(3),
			},
		},
	}
	data, _ := json.Marshal(src)
	if err := os.WriteFile(srcPath, data, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := Run(context.Background(), []string{"import", "--config", cfgPath, srcPath}); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Read the updated config.yaml and verify fields were preserved.
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	// Basic string checks — avoid importing yaml parser in test.
	cfgStr := string(cfgData)
	checks := []struct {
		field string
		want  string
	}{
		{"env API_KEY", "API_KEY"},
		{"env value", "secret123"},
		{"headers X-Custom", "X-Custom"},
		{"lazy", "lazy: true"},
		{"max_instances", "max_instances: 3"},
	}
	for _, c := range checks {
		found := false
		for _, line := range splitLines(cfgStr) {
			if containsStr(line, c.want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s (%q) to appear in config.yaml:\n%s", c.field, c.want, cfgStr)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
