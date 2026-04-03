package hub_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func postJSONRaw(client *http.Client, url, sessionID, payload string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	return client.Do(req)
}

// TestClientScopedRoutesWork verifies that /clients/<id>/mcp/<server> routes
// are accepted and process requests correctly (Phase 4).
func TestClientScopedRoutesWork(t *testing.T) {
	_, baseURL := newTestHub(t)

	client := &http.Client{Timeout: 5 * time.Second}

	// Initialize via client-scoped route.
	sessionID := initializeSessionViaURL(t, client, baseURL+"/clients/cursor/mcp/fixture")

	// Post a tools/list request via the client-scoped route.
	resp, err := postJSONRaw(client, baseURL+"/clients/cursor/mcp/fixture", sessionID,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if err != nil {
		t.Fatalf("tools/list via client route: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
}

// TestClientScopedRouteTracksClientID verifies that the hub status reflects
// per-client session counts after requests via client-scoped routes (Phase 4).
func TestClientScopedRouteTracksClientID(t *testing.T) {
	h, baseURL := newTestHub(t)
	defer h.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// Initialize two sessions: one for "cursor", one for "codex".
	initializeSessionViaURL(t, client, baseURL+"/clients/cursor/mcp/fixture")
	initializeSessionViaURL(t, client, baseURL+"/clients/codex/mcp/fixture")

	// Check status — both clients should appear.
	status := h.Status()
	if len(status.Servers) == 0 {
		t.Fatal("no servers in status")
	}
	srv := status.Servers[0]

	clientMap := map[string]int{}
	for _, cs := range srv.ClientSessions {
		clientMap[cs.ClientID] = cs.SessionCount
	}
	if clientMap["cursor"] == 0 {
		t.Errorf("expected cursor client session in status, got: %v", srv.ClientSessions)
	}
	if clientMap["codex"] == 0 {
		t.Errorf("expected codex client session in status, got: %v", srv.ClientSessions)
	}
}

func initializeSessionViaURL(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
	resp, err := postJSONRaw(client, url, "", initReq)
	if err != nil {
		t.Fatalf("initialize via %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize via %s: status %d", url, resp.StatusCode)
	}
	return resp.Header.Get("Mcp-Session-Id")
}
