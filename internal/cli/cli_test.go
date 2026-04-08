package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	testConfigFile = "config.yaml"
	testFlagConfig = "-config"
)

func TestIDEAddPreservesHubAPIKeyPlaceholderOnDisk(t *testing.T) {
	const (
		envName  = "MCP_HUB_API_KEY"
		resolved = "must-not-appear-in-yaml"
	)
	t.Setenv(envName, resolved)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, testConfigFile)
	raw := fmt.Sprintf(`hub:
  host: 127.0.0.1
  port: 4117
  api_key: "${%s}"
servers:
  keep:
    command: /bin/true
`, envName)
	if err := os.WriteFile(cfgPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := Run(ctx, []string{"ide", "add", testFlagConfig, cfgPath, "-path", "/tmp/out.json", "myide"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(after)
	if !strings.Contains(s, fmt.Sprintf("${%s}", envName)) {
		t.Fatalf("expected api_key placeholder to remain on disk; got:\n%s", s)
	}
	if strings.Contains(s, resolved) {
		t.Fatalf("expanded api_key leaked into config file")
	}
}

func TestAddServerPreservesHubAPIKeyPlaceholderOnDisk(t *testing.T) {
	const (
		envName  = "MCP_HUB_API_KEY_ADD"
		resolved = "secret-for-add-test"
	)
	t.Setenv(envName, resolved)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, testConfigFile)
	raw := fmt.Sprintf(`hub:
  host: 127.0.0.1
  port: 4117
  api_key: "${%s}"
servers:
  keep:
    command: /bin/true
`, envName)
	if err := os.WriteFile(cfgPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := Run(ctx, []string{"add", testFlagConfig, cfgPath, "-command", "/bin/true", "extra"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(after)
	if !strings.Contains(s, fmt.Sprintf("${%s}", envName)) {
		t.Fatalf("expected api_key placeholder to remain on disk; got:\n%s", s)
	}
	if strings.Contains(s, resolved) {
		t.Fatalf("expanded api_key leaked into config file")
	}
}

func TestStatusSendsExpandedAPIKeyInAuthorization(t *testing.T) {
	const (
		envName = "MCP_HUB_STATUS_TEST_TOKEN"
		token   = "resolved-bearer-token-abc"
	)
	t.Setenv(envName, token)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	var (
		mu      sync.Mutex
		gotAuth string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"0.test","address":"` + host + `:` + portStr + `","servers":[]}`))
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, testConfigFile)
	raw := fmt.Sprintf(`hub:
  host: %s
  port: %d
  api_key: "${%s}"
servers:
  s1:
    command: /bin/true
`, host, portNum, envName)

	if err := os.WriteFile(cfgPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := Run(ctx, []string{"status", testFlagConfig, cfgPath}); err != nil {
		t.Fatalf("Run status: %v", err)
	}

	mu.Lock()
	auth := gotAuth
	mu.Unlock()
	want := "Bearer " + token
	if auth != want {
		t.Fatalf("Authorization header = %q, want %q", auth, want)
	}
}

func TestStatusPrintsAggregateCounters(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version":"0.test",
			"address":"` + host + `:` + portStr + `",
			"aggregate_sessions":3,
			"aggregate_initialized":2,
			"aggregate_init_in_progress":1,
			"aggregate_initialization_errors":1,
			"servers":[]
		}`))
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, testConfigFile)
	raw := fmt.Sprintf(`hub:
  host: %s
  port: %d
servers:
  s1:
    command: /bin/true
`, host, portNum)
	if err := os.WriteFile(cfgPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	ctx := context.Background()
	runErr := Run(ctx, []string{"status", testFlagConfig, cfgPath})
	_ = w.Close()
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read captured output: %v", readErr)
	}
	if runErr != nil {
		t.Fatalf("Run status: %v", runErr)
	}

	got := string(out)
	want := "Aggregate sessions: 3 (initialized: 2, initializing: 1, init_errors: 1)"
	if !strings.Contains(got, want) {
		t.Fatalf("status output missing aggregate counters\nwant substring: %q\ngot:\n%s", want, got)
	}
}
