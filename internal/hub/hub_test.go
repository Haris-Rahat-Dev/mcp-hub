package hub_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/hub"
	"github.com/harisbinrahat/mcp-hub/internal/transport"
)

var (
	fixtureOnce sync.Once
	fixturePath string
	fixtureErr  error
)

func TestHubRoutesSharedRequestsAndNotifications(t *testing.T) {
	testHub, baseURL := newTestHub(t)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	sessionA := initializeSession(t, client, baseURL, "alpha")
	sessionB := initializeSession(t, client, baseURL, "beta")

	req, err := http.NewRequest(http.MethodGet, baseURL+"/mcp/fixture", nil)
	if err != nil {
		t.Fatalf("build stream request: %v", err)
	}
	req.Header.Set("Mcp-Session-Id", sessionA)
	streamResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer streamResp.Body.Close()

	type callResult struct {
		body []byte
		err  error
	}
	outA := make(chan callResult, 1)
	outB := make(chan callResult, 1)

	go func() {
		body, err := postJSON(client, baseURL+"/mcp/fixture", sessionA, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo","arguments":{"value":"from-a"}}}`)
		outA <- callResult{body: body, err: err}
	}()
	go func() {
		body, err := postJSON(client, baseURL+"/mcp/fixture", sessionB, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo","arguments":{"value":"from-b"}}}`)
		outB <- callResult{body: body, err: err}
	}()

	resultA := <-outA
	resultB := <-outB
	if resultA.err != nil {
		t.Fatalf("call A failed: %v", resultA.err)
	}
	if resultB.err != nil {
		t.Fatalf("call B failed: %v", resultB.err)
	}

	assertResponseHasEcho(t, resultA.body, 7, "from-a")
	assertResponseHasEcho(t, resultB.body, 7, "from-b")

	if _, err := postJSON(client, baseURL+"/mcp/fixture", sessionA, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"notify","arguments":{"value":"hello-stream"}}}`); err != nil {
		t.Fatalf("notify call failed: %v", err)
	}

	streamMessage := readStreamData(t, streamResp.Body)
	if !strings.Contains(streamMessage, "hello-stream") {
		t.Fatalf("expected notification payload in stream, got %q", streamMessage)
	}

	statusReq, err := http.NewRequest(http.MethodGet, baseURL+"/status", nil)
	if err != nil {
		t.Fatalf("build status request: %v", err)
	}
	statusResp, err := client.Do(statusReq)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer statusResp.Body.Close()

	var snapshot hub.StatusSnapshot
	if err := json.NewDecoder(statusResp.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(snapshot.Servers) != 1 || snapshot.Servers[0].Status != "running" {
		t.Fatalf("expected running fixture server, got %#v", snapshot.Servers)
	}
	if snapshot.Servers[0].Clients < 2 {
		t.Fatalf("expected two tracked sessions, got %d", snapshot.Servers[0].Clients)
	}
}

func TestHubRestartsSharedServerAfterCrash(t *testing.T) {
	testHub, baseURL := newTestHub(t)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	sessionID := initializeSession(t, client, baseURL, "restart")

	if _, err := postJSON(client, baseURL+"/mcp/fixture", sessionID, `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"crash","arguments":{}}}`); err != nil {
		t.Fatalf("crash call failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body, err := postJSON(client, baseURL+"/mcp/fixture", sessionID, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"echo","arguments":{"value":"after-restart"}}}`)
		if err == nil && strings.Contains(string(body), "after-restart") {
			statusReq, reqErr := http.NewRequest(http.MethodGet, baseURL+"/status", nil)
			if reqErr != nil {
				t.Fatalf("build status request: %v", reqErr)
			}
			statusResp, reqErr := client.Do(statusReq)
			if reqErr != nil {
				t.Fatalf("status request failed: %v", reqErr)
			}
			defer statusResp.Body.Close()
			var snapshot hub.StatusSnapshot
			if err := json.NewDecoder(statusResp.Body).Decode(&snapshot); err != nil {
				t.Fatalf("decode status: %v", err)
			}
			if len(snapshot.Servers) != 1 {
				t.Fatalf("expected one server status, got %#v", snapshot.Servers)
			}
			s := snapshot.Servers[0]
			if s.Restarts > 0 && s.LastRestartAt == "" {
				t.Fatalf("expected last_restart_at when restarts > 0, got %#v", s)
			}
			if s.UptimeSec < 0 {
				t.Fatalf("expected non-negative uptime, got %d", s.UptimeSec)
			}
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("fixture server did not recover before deadline")
}

func newTestHub(t *testing.T) (*hub.Hub, string) {
	t.Helper()

	return newTestHubWithMode(t, config.ModeShared)
}

func newTestHubWithMode(t *testing.T, mode config.Mode) (*hub.Hub, string) {
	t.Helper()

	lazy := false
	cfg := config.Config{
		Path: filepath.Join(t.TempDir(), "config.yaml"),
		Hub: config.HubConfig{
			Host:    "127.0.0.1",
			Port:    4117,
			LogFile: filepath.Join(t.TempDir(), "hub.log"),
		},
		Servers: map[string]config.ServerConfig{
			"fixture": {
				Command: fixtureBinary(t),
				Mode:    mode,
				Lazy:    &lazy,
			},
		},
	}

	logger := log.New(io.Discard, "", 0)
	testHub := hub.New(cfg, logger)
	if err := testHub.StartEager(context.Background()); err != nil {
		t.Fatalf("start eager: %v", err)
	}

	server := httptest.NewServer(transport.NewHandler(testHub, logger))
	t.Cleanup(server.Close)

	return testHub, server.URL
}

func newTestHubWithServer(t *testing.T, serverConfig config.ServerConfig) (*hub.Hub, string) {
	t.Helper()

	return newTestHubWithServers(t, map[string]config.ServerConfig{
		"fixture": serverConfig,
	})
}

func newTestHubWithServers(t *testing.T, servers map[string]config.ServerConfig) (*hub.Hub, string) {
	t.Helper()

	cfg := config.Config{
		Path: filepath.Join(t.TempDir(), "config.yaml"),
		Hub: config.HubConfig{
			Host:    "127.0.0.1",
			Port:    4117,
			LogFile: filepath.Join(t.TempDir(), "hub.log"),
		},
		Servers: servers,
	}

	logger := log.New(io.Discard, "", 0)
	testHub := hub.New(cfg, logger)
	if err := testHub.StartEager(context.Background()); err != nil {
		t.Fatalf("start eager: %v", err)
	}

	server := httptest.NewServer(transport.NewHandler(testHub, logger))
	t.Cleanup(server.Close)

	return testHub, server.URL
}

func TestHubRoutesIsolatedRequestsIndependently(t *testing.T) {
	testHub, baseURL := newTestHubWithMode(t, config.ModeIsolated)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	sessionA := initializeSession(t, client, baseURL, "alpha")
	sessionB := initializeSession(t, client, baseURL, "beta")

	bodyA, err := postJSON(client, baseURL+"/mcp/fixture", sessionA, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo","arguments":{"value":"from-a"}}}`)
	if err != nil {
		t.Fatalf("call A failed: %v", err)
	}
	bodyB, err := postJSON(client, baseURL+"/mcp/fixture", sessionB, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo","arguments":{"value":"from-b"}}}`)
	if err != nil {
		t.Fatalf("call B failed: %v", err)
	}

	assertResponseHasEcho(t, bodyA, 7, "from-a")
	assertResponseHasEcho(t, bodyB, 7, "from-b")
}

func TestHubScopesNotificationsToIsolatedSession(t *testing.T) {
	testHub, baseURL := newTestHubWithMode(t, config.ModeIsolated)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	sessionA := initializeSession(t, client, baseURL, "alpha")
	sessionB := initializeSession(t, client, baseURL, "beta")

	streamA := openStream(t, client, baseURL, sessionA)
	defer streamA.Body.Close()
	streamB := openStream(t, client, baseURL, sessionB)
	defer streamB.Body.Close()

	if _, err := postJSON(client, baseURL+"/mcp/fixture", sessionA, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"notify","arguments":{"value":"hello-a"}}}`); err != nil {
		t.Fatalf("notify call failed: %v", err)
	}

	gotA := readStreamData(t, streamA.Body)
	if !strings.Contains(gotA, "hello-a") {
		t.Fatalf("expected notification in stream A, got %q", gotA)
	}

	if msg, ok := readStreamDataMaybe(streamB.Body, 500*time.Millisecond); ok {
		t.Fatalf("expected no notification in stream B, got %q", msg)
	}
}

func TestHubIsolatedCrashDoesNotAffectOtherSession(t *testing.T) {
	testHub, baseURL := newTestHubWithMode(t, config.ModeIsolated)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	sessionA := initializeSession(t, client, baseURL, "alpha")
	sessionB := initializeSession(t, client, baseURL, "beta")

	if _, err := postJSON(client, baseURL+"/mcp/fixture", sessionA, `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"crash","arguments":{}}}`); err != nil {
		t.Fatalf("crash call failed: %v", err)
	}

	bodyB, err := postJSON(client, baseURL+"/mcp/fixture", sessionB, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"echo","arguments":{"value":"still-alive"}}}`)
	if err != nil {
		t.Fatalf("call B failed during A restart: %v", err)
	}
	assertResponseHasEcho(t, bodyB, 10, "still-alive")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		bodyA, err := postJSON(client, baseURL+"/mcp/fixture", sessionA, `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"echo","arguments":{"value":"after-restart"}}}`)
		if err == nil && strings.Contains(string(bodyA), "after-restart") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("session A did not recover before deadline")
}

func TestHubDeleteSessionTearsDownIsolatedState(t *testing.T) {
	testHub, baseURL := newTestHubWithMode(t, config.ModeIsolated)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	sessionA := initializeSession(t, client, baseURL, "alpha")

	if err := deleteSession(client, baseURL+"/mcp/fixture", sessionA); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}

	// Immediately after deletion, the server should have no tracked sessions.
	assertStatusClients(t, client, baseURL, 0)

	// After deletion, the session id may be reused but must re-initialize before requests succeed.
	body, err := postJSON(client, baseURL+"/mcp/fixture", sessionA, `{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"echo","arguments":{"value":"nope"}}}`)
	if err != nil {
		t.Fatalf("unexpected transport error calling without re-initialize: %v", err)
	}
	assertResponseHasRPCError(t, body, 12, -32002)
}

func TestHubRoutesReverseRequestForIsolatedSession(t *testing.T) {
	testHub, baseURL := newTestHubWithMode(t, config.ModeIsolated)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	sessionID := initializeSession(t, client, baseURL, "alpha")
	streamResp := openStream(t, client, baseURL, sessionID)
	defer streamResp.Body.Close()

	type callResult struct {
		body []byte
		err  error
	}
	callCh := make(chan callResult, 1)
	go func() {
		body, err := postJSON(client, baseURL+"/mcp/fixture", sessionID, `{"jsonrpc":"2.0","id":21,"method":"tools/call","params":{"name":"reverse","arguments":{}}}`)
		callCh <- callResult{body: body, err: err}
	}()

	streamData := readStreamDataMatchingMethod(t, streamResp.Body, "roots/list")
	reverseID := extractRequestID(t, streamData)

	replyPayload := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"roots":[{"uri":"file:///tmp/project"}]}}`, reverseID)
	if _, err := postJSON(client, baseURL+"/mcp/fixture", sessionID, replyPayload); err != nil {
		t.Fatalf("reply to reverse request failed: %v", err)
	}

	result := <-callCh
	if result.err != nil {
		t.Fatalf("reverse tools/call failed: %v", result.err)
	}
	if !strings.Contains(string(result.body), "file:///tmp/project") {
		t.Fatalf("expected reverse response to round-trip, got %s", result.body)
	}
}

func TestHubRoutesReverseRequestForSharedSession(t *testing.T) {
	testHub, baseURL := newTestHubWithMode(t, config.ModeShared)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	sessionID := initializeSession(t, client, baseURL, "alpha")
	streamResp := openStream(t, client, baseURL, sessionID)
	defer streamResp.Body.Close()

	type callResult struct {
		body []byte
		err  error
	}
	callCh := make(chan callResult, 1)
	go func() {
		body, err := postJSON(client, baseURL+"/mcp/fixture", sessionID, `{"jsonrpc":"2.0","id":31,"method":"tools/call","params":{"name":"reverse","arguments":{}}}`)
		callCh <- callResult{body: body, err: err}
	}()

	streamData := readStreamDataMatchingMethod(t, streamResp.Body, "roots/list")
	reverseID := extractRequestID(t, streamData)

	replyPayload := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"roots":[{"uri":"file:///tmp/shared"}]}}`, reverseID)
	if _, err := postJSON(client, baseURL+"/mcp/fixture", sessionID, replyPayload); err != nil {
		t.Fatalf("reply to reverse request failed: %v", err)
	}

	result := <-callCh
	if result.err != nil {
		t.Fatalf("reverse tools/call failed: %v", result.err)
	}
	if !strings.Contains(string(result.body), "file:///tmp/shared") {
		t.Fatalf("expected reverse response to round-trip, got %s", result.body)
	}
}

func TestSharedReverseRequestUnblocksOnDelete(t *testing.T) {
	testHub, baseURL := newTestHubWithMode(t, config.ModeShared)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	sessionID := initializeSession(t, client, baseURL, "alpha")
	streamResp := openStream(t, client, baseURL, sessionID)
	defer streamResp.Body.Close()

	type callResult struct {
		body []byte
		err  error
	}
	callCh := make(chan callResult, 1)
	go func() {
		body, err := postJSON(client, baseURL+"/mcp/fixture", sessionID, `{"jsonrpc":"2.0","id":41,"method":"tools/call","params":{"name":"reverse","arguments":{}}}`)
		callCh <- callResult{body: body, err: err}
	}()

	_ = readStreamDataMatchingMethod(t, streamResp.Body, "roots/list")

	if err := deleteSession(client, baseURL+"/mcp/fixture", sessionID); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}

	select {
	case result := <-callCh:
		if result.err != nil {
			t.Fatalf("reverse tools/call transport failed: %v", result.err)
		}
		if !strings.Contains(string(result.body), `"reverse_response":null`) {
			t.Fatalf("expected shared reverse call to unblock with null reverse_response after delete, got %s", result.body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shared reverse request did not unblock after DeleteSession")
	}
}

func TestHubPassthroughProxiesPostStreamDelete(t *testing.T) {
	var mu sync.Mutex
	upstreamSessionID := "upstream-session-1"
	var sawAuthHeader string
	var sawDeleteSession string
	var sawPostWithSession string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sawAuthHeader = r.Header.Get("Authorization")
		mu.Unlock()

		switch r.Method {
		case http.MethodPost:
			currentSID := r.Header.Get("Mcp-Session-Id")
			if currentSID == "" {
				w.Header().Set("Mcp-Session-Id", upstreamSessionID)
			} else {
				mu.Lock()
				sawPostWithSession = currentSID
				mu.Unlock()
			}
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Mcp-Session-Id", upstreamSessionID)
			_, _ = io.WriteString(w, "event: message\n")
			_, _ = io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/message\",\"params\":{\"level\":\"info\",\"data\":\"upstream\"}}\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case http.MethodDelete:
			mu.Lock()
			sawDeleteSession = r.Header.Get("Mcp-Session-Id")
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer upstream.Close()

	serverConfig := config.ServerConfig{
		URL:  upstream.URL,
		Mode: config.ModePassthrough,
		Headers: map[string]string{
			"Authorization": "Bearer test-token",
		},
	}
	testHub, baseURL := newTestHubWithServer(t, serverConfig)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	initReq, err := http.NewRequest(http.MethodPost, baseURL+"/mcp/fixture", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatalf("build init request: %v", err)
	}
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := client.Do(initReq)
	if err != nil {
		t.Fatalf("send init request: %v", err)
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(initResp.Body)
		t.Fatalf("init status=%d body=%s", initResp.StatusCode, body)
	}
	localSessionID := initResp.Header.Get("Mcp-Session-Id")
	if localSessionID == "" {
		t.Fatalf("expected hub to return local session id")
	}

	if _, err := postJSON(client, baseURL+"/mcp/fixture", localSessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"value":"x"}}}`); err != nil {
		t.Fatalf("passthrough post failed: %v", err)
	}

	streamResp := openStream(t, client, baseURL, localSessionID)
	defer streamResp.Body.Close()
	streamData := readStreamData(t, streamResp.Body)
	if !strings.Contains(streamData, `"upstream"`) {
		t.Fatalf("expected upstream SSE data, got %q", streamData)
	}

	if err := deleteSession(client, baseURL+"/mcp/fixture", localSessionID); err != nil {
		t.Fatalf("delete passthrough session failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if sawAuthHeader != "Bearer test-token" {
		t.Fatalf("expected passthrough auth header, got %q", sawAuthHeader)
	}
	if sawPostWithSession != upstreamSessionID {
		t.Fatalf("expected follow-up post to carry upstream session id %q, got %q", upstreamSessionID, sawPostWithSession)
	}
	if sawDeleteSession != upstreamSessionID {
		t.Fatalf("expected delete to carry upstream session id %q, got %q", upstreamSessionID, sawDeleteSession)
	}
}

func TestHubAggregatedEndpointNamespacedTools(t *testing.T) {
	testHub, baseURL := newTestHubWithMode(t, config.ModeShared)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	initReq, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatalf("build init request: %v", err)
	}
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := client.Do(initReq)
	if err != nil {
		t.Fatalf("send init request: %v", err)
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(initResp.Body)
		t.Fatalf("init status=%d body=%s", initResp.StatusCode, body)
	}
	sessionID := initResp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatalf("expected aggregated session id")
	}

	if _, err := postJSON(client, baseURL+"/mcp", sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`); err != nil {
		t.Fatalf("initialized notification failed: %v", err)
	}

	toolsResp, err := postJSON(client, baseURL+"/mcp", sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	if !strings.Contains(string(toolsResp), `"fixture__echo"`) {
		t.Fatalf("expected namespaced tool in aggregated tools/list, got %s", toolsResp)
	}

	callResp, err := postJSON(client, baseURL+"/mcp", sessionID, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"fixture__echo","arguments":{"value":"agg"}}}`)
	if err != nil {
		t.Fatalf("aggregated tools/call failed: %v", err)
	}
	assertResponseHasEcho(t, callResp, 3, "agg")
}

func TestHubAggregatedInitializeFailsWhenAnyBackendFails(t *testing.T) {
	testHub, baseURL := newTestHubWithServers(t, map[string]config.ServerConfig{
		"broken": {
			Command: filepath.Join(t.TempDir(), "missing-binary"),
			Mode:    config.ModeShared,
		},
		"fixture": {
			Command: fixtureBinary(t),
			Mode:    config.ModeShared,
		},
	})
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	initReq, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatalf("build init request: %v", err)
	}
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := client.Do(initReq)
	if err != nil {
		t.Fatalf("send init request: %v", err)
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(initResp.Body)
		t.Fatalf("init status=%d body=%s", initResp.StatusCode, body)
	}
	sessionID := initResp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatalf("expected aggregated session id")
	}
	body, err := io.ReadAll(initResp.Body)
	if err != nil {
		t.Fatalf("read init response: %v", err)
	}
	assertResponseHasRPCError(t, body, 1, -32000)
	if !strings.Contains(string(body), "broken") {
		t.Fatalf("expected failing backend name in init error, got %s", body)
	}

	if _, err := postJSON(client, baseURL+"/mcp", sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`); err != nil {
		t.Fatalf("initialized notification failed: %v", err)
	}

	toolsResp, err := postJSON(client, baseURL+"/mcp", sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	assertResponseHasRPCError(t, toolsResp, 2, -32002)
}

func TestHubAggregatedInitializeConcurrentFailureReturnsErrorForWaiters(t *testing.T) {
	testHub, baseURL := newTestHubWithServers(t, map[string]config.ServerConfig{
		"broken": {
			Command: filepath.Join(t.TempDir(), "missing-binary"),
			Mode:    config.ModeShared,
		},
		"fixture": {
			Command: fixtureBinary(t),
			Mode:    config.ModeShared,
		},
	})
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	sessionID := "agg-race-fail"
	payload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`

	call := func() []byte {
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(payload))
		if err != nil {
			t.Fatalf("build init request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Mcp-Session-Id", sessionID)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("send init request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("init status=%d body=%s", resp.StatusCode, body)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read init response: %v", err)
		}
		return body
	}

	bodyCh := make(chan []byte, 2)
	go func() { bodyCh <- call() }()
	go func() { bodyCh <- call() }()

	first := <-bodyCh
	second := <-bodyCh

	assertResponseHasRPCError(t, first, 1, -32000)
	assertResponseHasRPCError(t, second, 1, -32000)
	if strings.Contains(string(first), `"result"`) || strings.Contains(string(second), `"result"`) {
		t.Fatalf("expected both concurrent initialize calls to fail, got first=%s second=%s", first, second)
	}
}

func TestHubStatusReportsAggregateSessionCounters(t *testing.T) {
	testHub, baseURL := newTestHubWithMode(t, config.ModeShared)
	defer testHub.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	initReq, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatalf("build init request: %v", err)
	}
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := client.Do(initReq)
	if err != nil {
		t.Fatalf("send init request: %v", err)
	}
	defer initResp.Body.Close()
	if initResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(initResp.Body)
		t.Fatalf("init status=%d body=%s", initResp.StatusCode, body)
	}
	sessionID := initResp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatalf("expected aggregate session id")
	}

	statusReq, err := http.NewRequest(http.MethodGet, baseURL+"/status", nil)
	if err != nil {
		t.Fatalf("build status request: %v", err)
	}
	statusResp, err := client.Do(statusReq)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer statusResp.Body.Close()
	var snap hub.StatusSnapshot
	if err := json.NewDecoder(statusResp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if snap.AggregateSessions != 1 || snap.AggregateInitialized != 1 || snap.AggregateInitInProgress != 0 {
		t.Fatalf("unexpected aggregate counters: %#v", snap)
	}

	if err := deleteSession(client, baseURL+"/mcp", sessionID); err != nil {
		t.Fatalf("delete aggregate session: %v", err)
	}

	statusResp2, err := client.Do(statusReq)
	if err != nil {
		t.Fatalf("status request after delete failed: %v", err)
	}
	defer statusResp2.Body.Close()
	var snap2 hub.StatusSnapshot
	if err := json.NewDecoder(statusResp2.Body).Decode(&snap2); err != nil {
		t.Fatalf("decode status after delete: %v", err)
	}
	if snap2.AggregateSessions != 0 || snap2.AggregateInitialized != 0 || snap2.AggregateInitializationErr != 0 {
		t.Fatalf("expected cleared aggregate counters after delete, got %#v", snap2)
	}
}

func initializeSession(t *testing.T, client *http.Client, baseURL, value string) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp/fixture", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatalf("build initialize request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("initialize request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("initialize status=%d body=%s", resp.StatusCode, body)
	}

	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatalf("initialize did not return session id")
	}

	if _, err := postJSON(client, baseURL+"/mcp/fixture", sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized","params":{"value":"`+value+`"}}`); err != nil {
		t.Fatalf("initialized notification failed: %v", err)
	}

	return sessionID
}

func postJSON(client *http.Client, url, sessionID, payload string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, &unexpectedStatusError{statusCode: resp.StatusCode, body: string(body)}
	}
	return body, nil
}

func openStream(t *testing.T, client *http.Client, baseURL, sessionID string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/mcp/fixture", nil)
	if err != nil {
		t.Fatalf("build stream request: %v", err)
	}
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("stream status=%d body=%s", resp.StatusCode, body)
	}
	return resp
}

func deleteSession(client *http.Client, url, sessionID string) error {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return &unexpectedStatusError{statusCode: resp.StatusCode, body: string(body)}
	}
	return nil
}

func assertResponseHasEcho(t *testing.T, body []byte, wantID int, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if int(payload["id"].(float64)) != wantID {
		t.Fatalf("expected original id %d to survive remap, got %#v", wantID, payload["id"])
	}
	result := payload["result"].(map[string]any)
	echo := result["echo"].(map[string]any)
	if got := echo["value"].(string); got != want {
		t.Fatalf("expected echoed value %q, got %q", want, got)
	}
}

func assertResponseHasRPCError(t *testing.T, body []byte, wantID int, wantCode int) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if int(payload["id"].(float64)) != wantID {
		t.Fatalf("expected id %d, got %#v", wantID, payload["id"])
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %#v", payload)
	}
	if int(errObj["code"].(float64)) != wantCode {
		t.Fatalf("expected error code %d, got %#v", wantCode, errObj["code"])
	}
}

func assertStatusClients(t *testing.T, client *http.Client, baseURL string, want int) {
	t.Helper()

	statusReq, err := http.NewRequest(http.MethodGet, baseURL+"/status", nil)
	if err != nil {
		t.Fatalf("build status request: %v", err)
	}
	statusResp, err := client.Do(statusReq)
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}
	defer statusResp.Body.Close()

	var snapshot hub.StatusSnapshot
	if err := json.NewDecoder(statusResp.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(snapshot.Servers) != 1 || snapshot.Servers[0].Clients != want {
		t.Fatalf("expected %d tracked sessions, got %#v", want, snapshot.Servers)
	}
}

func extractRequestID(t *testing.T, jsonPayload string) string {
	t.Helper()
	var msg map[string]any
	if err := json.Unmarshal([]byte(jsonPayload), &msg); err != nil {
		t.Fatalf("parse reverse request payload: %v payload=%s", err, jsonPayload)
	}
	id, ok := msg["id"]
	if !ok {
		t.Fatalf("reverse request payload missing id: %s", jsonPayload)
	}
	switch v := id.(type) {
	case string:
		encoded, _ := json.Marshal(v)
		return string(encoded)
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		t.Fatalf("unsupported reverse request id type %T", id)
		return ""
	}
}

func readStreamData(t *testing.T, body io.Reader) string {
	t.Helper()

	reader := bufio.NewReader(body)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream line: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		}
	}
	t.Fatalf("timed out waiting for stream data")
	return ""
}

func readStreamDataMaybe(body io.Reader, timeout time.Duration) (string, bool) {
	type result struct {
		msg string
		ok  bool
	}
	out := make(chan result, 1)

	go func() {
		reader := bufio.NewReader(body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				out <- result{msg: "", ok: false}
				return
			}
			if strings.HasPrefix(line, "data: ") {
				out <- result{msg: strings.TrimSpace(strings.TrimPrefix(line, "data: ")), ok: true}
				return
			}
		}
	}()

	select {
	case res := <-out:
		return res.msg, res.ok
	case <-time.After(timeout):
		return "", false
	}
}

func readStreamDataMatchingMethod(t *testing.T, body io.Reader, method string) string {
	t.Helper()
	reader := bufio.NewReader(body)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream line: %v", err)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		var msg map[string]any
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			continue
		}
		if got, _ := msg["method"].(string); got == method {
			return payload
		}
	}
	t.Fatalf("timed out waiting for stream method %q", method)
	return ""
}

func fixtureBinary(t *testing.T) string {
	t.Helper()

	fixtureOnce.Do(func() {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			fixtureErr = os.ErrNotExist
			return
		}
		root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		binDir, err := os.MkdirTemp("", "mcp-hub-fixture-*")
		if err != nil {
			fixtureErr = err
			return
		}
		fixturePath = filepath.Join(binDir, "fixture")
		cmd := exec.Command("go", "build", "-o", fixturePath, "./cmd/mcp-hub-fixture")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err != nil {
			fixtureErr = unexpectedStatusError{statusCode: -1, body: string(output)}
		}
	})

	if fixtureErr != nil {
		t.Fatalf("build fixture: %v", fixtureErr)
	}
	return fixturePath
}

type unexpectedStatusError struct {
	statusCode int
	body       string
}

func (e unexpectedStatusError) Error() string {
	return strings.TrimSpace(e.body)
}
