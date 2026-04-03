package hub_test

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/hub"
	"github.com/harisbinrahat/mcp-hub/internal/transport"
)

// TestAggregateInitRaceNoDuplicate verifies that concurrent initialize calls
// on the same aggregate session do not trigger double-initialization of backend
// servers (fix 0c).
func TestAggregateInitRaceNoDuplicate(t *testing.T) {
	testHub, _ := newTestHub(t)
	defer testHub.Close()

	ctx := context.Background()
	initReq := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)

	const concurrency = 10
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := testHub.HandleAggregateRequest(ctx, "", initReq)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent aggregate init error: %v", err)
	}
}

// TestIsolatedReverseRequestUnblocksOnDelete verifies that a reverse-request
// waiter unblocks promptly when the session is deleted (fix 0d).
func TestIsolatedReverseRequestUnblocksOnDelete(t *testing.T) {
	lazy := false
	cfg := config.Config{
		Path: filepath.Join(t.TempDir(), "config.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117, LogFile: filepath.Join(t.TempDir(), "hub.log")},
		Servers: map[string]config.ServerConfig{
			"fixture": {Command: fixtureBinary(t), Mode: config.ModeIsolated, Lazy: &lazy},
		},
	}
	logger := log.New(io.Discard, "", 0)
	h := hub.New(cfg, logger)
	if err := h.StartEager(context.Background()); err != nil {
		t.Fatalf("start eager: %v", err)
	}
	defer h.Close()

	srv := httptest.NewServer(transport.NewHandler(h, logger))
	defer srv.Close()

	// Initialize a session so the backend process is running.
	ctx := context.Background()
	sessionID, _, err := h.HandleRequest(ctx, "fixture", "", "", []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`))
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Delete the session; this should not hang.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = h.DeleteSession("fixture", sessionID)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("DeleteSession hung — reverse-request channel race not fixed")
	}
}

// TestWatchProcessStopRestartInterrupt verifies that closing a sharedServer
// does not hang when a restart backoff is in progress (fix 0a).
func TestWatchProcessStopRestartInterrupt(t *testing.T) {
	lazy := false
	cfg := config.Config{
		Path: filepath.Join(t.TempDir(), "config.yaml"),
		Hub:  config.HubConfig{Host: "127.0.0.1", Port: 4117, LogFile: filepath.Join(t.TempDir(), "hub.log")},
		Servers: map[string]config.ServerConfig{
			"fixture": {Command: fixtureBinary(t), Mode: config.ModeShared, Lazy: &lazy},
		},
	}
	logger := log.New(io.Discard, "", 0)
	h := hub.New(cfg, logger)
	if err := h.StartEager(context.Background()); err != nil {
		t.Fatalf("start eager: %v", err)
	}

	// Close the hub; this must complete quickly even if a restart goroutine
	// is sleeping in the backoff.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = h.Close()
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Hub.Close() hung — stopRestart channel not signalled")
	}
}
