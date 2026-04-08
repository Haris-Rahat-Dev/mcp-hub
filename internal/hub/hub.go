package hub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/multiplex"
	"github.com/harisbinrahat/mcp-hub/internal/process"
	"github.com/harisbinrahat/mcp-hub/internal/protocol"
)

const Version = "0.1.0"

var (
	ErrUnknownServer      = errors.New("unknown server")
	ErrModeNotImplemented = errors.New("server mode is not implemented")
)

type Hub struct {
	cfg         config.Config
	logger      *log.Logger
	serversMu   sync.RWMutex
	servers     map[string]serverRuntime
	mu          sync.Mutex
	aggInit     map[string]bool
	aggInitErr  map[string]string
	aggInitWait map[string]chan struct{}
	// clientSessions tracks which client ids are associated with which session ids.
	// Key: clientID → set of sessionIDs.
	clientSessions map[string]map[string]bool
	// sessionClient maps sessionID → clientID for reverse lookup.
	sessionClient map[string]string
}

type StatusSnapshot struct {
	Version                    string         `json:"version"`
	Address                    string         `json:"address"`
	Servers                    []ServerStatus `json:"servers"`
	AggregateSessions          int            `json:"aggregate_sessions"`
	AggregateInitialized       int            `json:"aggregate_initialized"`
	AggregateInitInProgress    int            `json:"aggregate_init_in_progress"`
	AggregateInitializationErr int            `json:"aggregate_initialization_errors"`
}

// ClientSessionCount reports how many sessions a specific client has on a server.
type ClientSessionCount struct {
	ClientID     string `json:"client_id"`
	SessionCount int    `json:"session_count"`
}

type ServerStatus struct {
	Name           string               `json:"name"`
	Mode           config.Mode          `json:"mode"`
	Status         string               `json:"status"`
	PID            int                  `json:"pid,omitempty"`
	Clients        int                  `json:"clients"`
	Restarts       int                  `json:"restarts"`
	UptimeSec      int64                `json:"uptime_sec,omitempty"`
	LastRestartAt  string               `json:"last_restart_at,omitempty"`
	LastError      string               `json:"last_error,omitempty"`
	Lazy           bool                 `json:"lazy"`
	LastPingMs     int64                `json:"last_ping_ms,omitempty"`
	LastPingAt     string               `json:"last_ping_at,omitempty"`
	ClientSessions []ClientSessionCount `json:"client_sessions,omitempty"`
}

type serverRuntime interface {
	ensureSessionID(existing string) string
	handleRequest(ctx context.Context, sessionID string, raw []byte) ([]byte, error)
	subscribe(sessionID string) (<-chan []byte, func())
	deleteSession(sessionID string)
	statusSnapshot() ServerStatus
	sessionIDs() []string
	close() error
}

type session struct {
	initialized bool
	nextSubID   uint64
	subscribers map[uint64]chan []byte
}

type reverseWaiter struct {
	sessionID string
	ch        chan []byte
}

type sharedServer struct {
	name       string
	def        config.ServerConfig
	logger     *log.Logger
	supervisor *process.Supervisor
	engine     *multiplex.Engine

	mu                   sync.Mutex
	process              *process.ManagedProcess
	initResult           json.RawMessage
	sessions             map[string]*session
	starting             bool
	startWait            chan struct{}
	status               string
	lastError            string
	restarts             int
	stopping             bool
	stopRestart          chan struct{}
	reverseReq           map[string]reverseWaiter
	progressByToken      map[string]string
	lastPingMs           int64
	lastPingAt           time.Time
	startedAt            time.Time
	lastRestartAt        time.Time
	requestMu            sync.Mutex
	activeReverseSession string
}

func (s *sharedServer) startEager(ctx context.Context) error {
	if s.def.Mode == config.ModeShared && !s.def.IsLazy() {
		return s.ensureStarted(ctx)
	}
	return nil
}

func New(cfg config.Config, logger *log.Logger) *Hub {
	h := &Hub{
		cfg:            cfg,
		logger:         logger,
		servers:        map[string]serverRuntime{},
		aggInit:        map[string]bool{},
		aggInitErr:     map[string]string{},
		aggInitWait:    map[string]chan struct{}{},
		clientSessions: map[string]map[string]bool{},
		sessionClient:  map[string]string{},
	}
	for name, def := range cfg.Servers {
		switch def.Mode {
		case config.ModeShared:
			h.servers[name] = newSharedServer(name, def, logger)
		case config.ModeIsolated:
			h.servers[name] = newIsolatedServer(name, def, logger)
		case config.ModePassthrough:
			h.servers[name] = newPassthroughServer(name, def, logger)
		default:
			h.servers[name] = newPlannedServer(name, def)
		}
	}
	return h
}

func (h *Hub) StartEager(ctx context.Context) error {
	names := h.sortedServerNames()
	for _, name := range names {
		server, ok := h.getServer(name)
		if !ok {
			continue
		}
		if eager, ok := server.(interface {
			startEager(ctx context.Context) error
		}); ok {
			if err := eager.startEager(ctx); err != nil {
				return fmt.Errorf("start %s: %w", name, err)
			}
		}
	}
	return nil
}

func (h *Hub) HandleRequest(ctx context.Context, serverName, sessionID, clientID string, raw []byte) (string, []byte, error) {
	server, ok := h.getServer(serverName)
	if !ok {
		return "", nil, ErrUnknownServer
	}
	sessionID = server.ensureSessionID(sessionID)
	h.recordClientSession(clientID, sessionID)
	response, err := server.handleRequest(ctx, sessionID, raw)
	return sessionID, response, err
}

func (h *Hub) Subscribe(serverName, sessionID, clientID string) (string, <-chan []byte, func(), error) {
	server, ok := h.getServer(serverName)
	if !ok {
		return "", nil, nil, ErrUnknownServer
	}
	sessionID = server.ensureSessionID(sessionID)
	h.recordClientSession(clientID, sessionID)
	ch, cancel := server.subscribe(sessionID)
	wrappedCancel := func() {
		cancel()
		h.removeClientSession(sessionID)
	}
	return sessionID, ch, wrappedCancel, nil
}

func (h *Hub) DeleteSession(serverName, sessionID string) error {
	server, ok := h.getServer(serverName)
	if !ok {
		return ErrUnknownServer
	}
	server.deleteSession(sessionID)
	h.removeClientSession(sessionID)
	return nil
}

func (h *Hub) HandleAggregateRequest(ctx context.Context, sessionID string, raw []byte) (string, []byte, error) {
	sessionID = h.ensureAggregateSessionID(sessionID)

	env, err := protocol.Parse(raw)
	if err != nil {
		return sessionID, nil, err
	}

	switch {
	case protocol.IsRequest(env) && env.Method == "initialize":
		// Serialize concurrent initialize calls for the same session to prevent
		// double-initialization of all backend servers (TOCTOU race).
		h.mu.Lock()
		if wait, inProgress := h.aggInitWait[sessionID]; inProgress {
			h.mu.Unlock()
			select {
			case <-wait:
			case <-ctx.Done():
				return sessionID, nil, ctx.Err()
			}
			// Another caller already finished initialize for this session.
			// Return success only if that initialize succeeded.
			h.mu.Lock()
			ok := h.aggInit[sessionID]
			initErr := h.aggInitErr[sessionID]
			h.mu.Unlock()
			if !ok {
				if initErr == "" {
					initErr = "aggregate initialize failed"
				}
				resp, err := protocol.NewErrorResponse(*env.ID, -32000, initErr)
				return sessionID, resp, err
			}
			result := json.RawMessage(`{"protocolVersion":"2025-03-26","capabilities":{"tools":{}},"serverInfo":{"name":"mcp-hub","version":"0.1.0"}}`)
			resp, err := protocol.NewSuccessResponse(*env.ID, result)
			return sessionID, resp, err
		}
		gate := make(chan struct{})
		h.aggInitWait[sessionID] = gate
		h.mu.Unlock()

		var merged map[string]any
		var initErr error
		var initResp []byte
		for _, serverName := range h.sortedServerNames() {
			_, resp, err := h.HandleRequest(ctx, serverName, sessionID, "", raw)
			if err != nil {
				initErr = err
				initResp, _ = protocol.NewErrorResponse(*env.ID, -32000, fmt.Sprintf("initialize %s: %v", serverName, err))
				break
			}
			if err := validateAggregateInitializeResponse(serverName, resp); err != nil {
				initErr = err
				initResp, _ = protocol.NewErrorResponse(*env.ID, -32000, err.Error())
				break
			}
			merged = mergeInitializeResult(merged, resp)
		}

		h.mu.Lock()
		delete(h.aggInitWait, sessionID)
		if initErr == nil {
			h.aggInit[sessionID] = true
			delete(h.aggInitErr, sessionID)
		} else {
			h.aggInit[sessionID] = false
			h.aggInitErr[sessionID] = initErr.Error()
		}
		close(gate)
		h.mu.Unlock()

		if initErr != nil {
			return sessionID, initResp, nil
		}
		result := json.RawMessage(`{"protocolVersion":"2025-03-26","capabilities":{"tools":{}},"serverInfo":{"name":"mcp-hub","version":"0.1.0"}}`)
		if len(merged) > 0 {
			if b, err := json.Marshal(merged); err == nil {
				result = b
			}
		}
		resp, err := protocol.NewSuccessResponse(*env.ID, result)
		return sessionID, resp, err
	case protocol.IsNotification(env) && isInitializedNotification(env.Method):
		if !h.aggregateInitialized(sessionID) {
			return sessionID, nil, nil
		}
		for _, serverName := range h.sortedServerNames() {
			_, _, _ = h.HandleRequest(ctx, serverName, sessionID, "", raw)
		}
		return sessionID, nil, nil
	}

	if !h.aggregateInitialized(sessionID) {
		if env.ID == nil {
			return sessionID, nil, nil
		}
		resp, err := protocol.NewErrorResponse(*env.ID, -32002, "session must initialize before making requests")
		return sessionID, resp, err
	}

	if !protocol.IsRequest(env) {
		return sessionID, nil, nil
	}

	switch env.Method {
	case "tools/list":
		resp, err := h.handleAggregateToolsList(ctx, sessionID, *env.ID)
		return sessionID, resp, err
	case "tools/call":
		resp, err := h.handleAggregateToolCall(ctx, sessionID, raw, env)
		return sessionID, resp, err
	// Additional aggregate methods can be added here.
	default:
		resp, err := protocol.NewErrorResponse(*env.ID, -32601, "method not supported on aggregated endpoint")
		return sessionID, resp, err
	}
}

func (h *Hub) SubscribeAggregate(sessionID string) (string, <-chan []byte, func(), error) {
	sessionID = h.ensureAggregateSessionID(sessionID)
	out := make(chan []byte, 32)
	cancels := make([]func(), 0, len(h.servers))
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for _, serverName := range h.sortedServerNames() {
		_, ch, cancel, err := h.Subscribe(serverName, sessionID, "")
		if err != nil {
			for _, c := range cancels {
				c()
			}
			close(out)
			return "", nil, nil, err
		}
		cancels = append(cancels, cancel)
		wg.Add(1)
		go func(src <-chan []byte) {
			defer wg.Done()
			for msg := range src {
				select {
				case <-stop:
					return
				case out <- msg:
				default:
				}
			}
		}(ch)
	}

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			close(stop)
			for _, c := range cancels {
				c()
			}
			wg.Wait()
			close(out)
		})
	}
	return sessionID, out, cancel, nil
}

func (h *Hub) DeleteAggregateSession(sessionID string) error {
	for _, serverName := range h.sortedServerNames() {
		_ = h.DeleteSession(serverName, sessionID)
	}
	h.mu.Lock()
	delete(h.aggInit, sessionID)
	delete(h.aggInitErr, sessionID)
	delete(h.aggInitWait, sessionID)
	h.mu.Unlock()
	return nil
}

func (h *Hub) Status() StatusSnapshot {
	names := h.sortedServerNames()
	servers := make([]ServerStatus, 0, len(names))
	for _, name := range names {
		if srv, ok := h.getServer(name); ok {
			snap := srv.statusSnapshot()
			snap.ClientSessions = h.clientSessionCounts(srv.sessionIDs())
			servers = append(servers, snap)
		}
	}

	h.mu.Lock()
	aggSessions := len(h.aggInit)
	aggInitialized := 0
	for _, initialized := range h.aggInit {
		if initialized {
			aggInitialized++
		}
	}
	aggInProgress := len(h.aggInitWait)
	aggErrors := len(h.aggInitErr)
	h.mu.Unlock()

	return StatusSnapshot{
		Version:                    Version,
		Address:                    h.cfg.Hub.ListenAddress(),
		Servers:                    servers,
		AggregateSessions:          aggSessions,
		AggregateInitialized:       aggInitialized,
		AggregateInitInProgress:    aggInProgress,
		AggregateInitializationErr: aggErrors,
	}
}

func (h *Hub) APIKey() string {
	return h.cfg.Hub.APIKey
}

func (h *Hub) ensureAggregateSessionID(existing string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing == "" {
		existing = newSessionID()
	}
	if _, ok := h.aggInit[existing]; !ok {
		h.aggInit[existing] = false
		delete(h.aggInitErr, existing)
	}
	return existing
}

func (h *Hub) aggregateInitialized(sessionID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.aggInit[sessionID]
}

func (h *Hub) recordClientSession(clientID, sessionID string) {
	if clientID == "" {
		clientID = "unknown"
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clientSessions[clientID] == nil {
		h.clientSessions[clientID] = map[string]bool{}
	}
	h.clientSessions[clientID][sessionID] = true
	h.sessionClient[sessionID] = clientID
}

func (h *Hub) removeClientSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clientID, ok := h.sessionClient[sessionID]
	if !ok {
		return
	}
	delete(h.sessionClient, sessionID)
	if sessions, ok := h.clientSessions[clientID]; ok {
		delete(sessions, sessionID)
		if len(sessions) == 0 {
			delete(h.clientSessions, clientID)
		}
	}
}

// clientSessionCounts returns a snapshot of per-client session counts for the
// given set of session IDs (typically all sessions on one server).
func (h *Hub) clientSessionCounts(sessionIDs []string) []ClientSessionCount {
	h.mu.Lock()
	defer h.mu.Unlock()
	counts := map[string]int{}
	for _, sid := range sessionIDs {
		if cid, ok := h.sessionClient[sid]; ok {
			counts[cid]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	result := make([]ClientSessionCount, 0, len(counts))
	for cid, n := range counts {
		result = append(result, ClientSessionCount{ClientID: cid, SessionCount: n})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ClientID < result[j].ClientID })
	return result
}

func (h *Hub) sortedServerNames() []string {
	h.serversMu.RLock()
	defer h.serversMu.RUnlock()
	names := make([]string, 0, len(h.servers))
	for name := range h.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (h *Hub) getServer(name string) (serverRuntime, bool) {
	h.serversMu.RLock()
	defer h.serversMu.RUnlock()
	s, ok := h.servers[name]
	return s, ok
}

func (h *Hub) handleAggregateToolsList(ctx context.Context, sessionID string, id json.RawMessage) ([]byte, error) {
	allTools := make([]map[string]any, 0)
	for _, serverName := range h.sortedServerNames() {
		raw := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
		_, response, err := h.HandleRequest(ctx, serverName, sessionID, "", raw)
		if err != nil || len(response) == 0 {
			continue
		}
		env, err := protocol.Parse(response)
		if err != nil || env.Result == nil {
			continue
		}
		var result map[string]any
		if err := json.Unmarshal(*env.Result, &result); err != nil {
			continue
		}
		toolsAny, ok := result["tools"].([]any)
		if !ok {
			continue
		}
		for _, item := range toolsAny {
			tool, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name, ok := tool["name"].(string)
			if !ok {
				continue
			}
			toolCopy := map[string]any{}
			for k, v := range tool {
				toolCopy[k] = v
			}
			toolCopy["name"] = serverName + "__" + name
			allTools = append(allTools, toolCopy)
		}
	}
	payload, err := json.Marshal(map[string]any{
		"tools": allTools,
	})
	if err != nil {
		return nil, err
	}
	return protocol.NewSuccessResponse(id, payload)
}

func (h *Hub) handleAggregateToolCall(ctx context.Context, sessionID string, raw []byte, env protocol.Envelope) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	params, ok := payload["params"].(map[string]any)
	if !ok {
		return protocol.NewErrorResponse(*env.ID, -32602, "tools/call requires params")
	}
	toolName, ok := params["name"].(string)
	if !ok || toolName == "" {
		return protocol.NewErrorResponse(*env.ID, -32602, "tools/call requires params.name")
	}
	serverName, innerTool, found := strings.Cut(toolName, "__")
	if !found || serverName == "" || innerTool == "" {
		return protocol.NewErrorResponse(*env.ID, -32602, "tools/call name must be namespaced as server__tool")
	}
	if _, ok := h.getServer(serverName); !ok {
		return protocol.NewErrorResponse(*env.ID, -32602, "unknown namespaced server")
	}
	params["name"] = innerTool
	payload["params"] = params
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	_, response, err := h.HandleRequest(ctx, serverName, sessionID, "", rewritten)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (h *Hub) Close() error {
	h.serversMu.Lock()
	current := h.servers
	h.servers = map[string]serverRuntime{}
	h.serversMu.Unlock()

	var firstErr error
	for _, server := range current {
		if err := server.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h *Hub) Reload(ctx context.Context, cfg config.Config) error {
	fresh := New(cfg, h.logger)
	if err := fresh.StartEager(ctx); err != nil {
		return err
	}

	h.serversMu.Lock()
	oldServers := h.servers
	h.servers = fresh.servers
	h.cfg = cfg
	h.serversMu.Unlock()

	for _, s := range oldServers {
		_ = s.close()
	}
	return nil
}

func newSharedServer(name string, def config.ServerConfig, logger *log.Logger) *sharedServer {
	server := &sharedServer{
		name:            name,
		def:             def,
		logger:          logger,
		supervisor:      &process.Supervisor{Logger: logger},
		sessions:        map[string]*session{},
		status:          "idle",
		stopRestart:     make(chan struct{}),
		reverseReq:      map[string]reverseWaiter{},
		progressByToken: map[string]string{},
	}
	if def.Mode != config.ModeShared {
		server.status = "planned"
	}
	server.engine = multiplex.NewEngine(server.broadcastNotification, server.handleReverseRequest)
	return server
}

func (s *sharedServer) ensureSessionID(existing string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing == "" {
		existing = newSessionID()
	}
	s.ensureSessionLocked(existing)
	return existing
}

func (s *sharedServer) handleRequest(ctx context.Context, sessionID string, raw []byte) ([]byte, error) {
	if s.def.Mode != config.ModeShared {
		return nil, ErrModeNotImplemented
	}

	env, err := protocol.Parse(raw)
	if err != nil {
		return nil, err
	}
	if protocol.IsResponse(env) && env.ID != nil {
		if s.resolveReverseRequest(idKey(*env.ID), raw) {
			return nil, nil
		}
	}
	if protocol.IsRequest(env) {
		if tok, ok := progressTokenFromRequest(raw); ok {
			s.mu.Lock()
			s.progressByToken[tok] = sessionID
			s.mu.Unlock()
		}
	}

	switch {
	case protocol.IsRequest(env) && env.Method == "initialize":
		if err := s.ensureStarted(ctx); err != nil {
			return protocol.NewErrorResponse(*env.ID, -32000, err.Error())
		}
		s.markSessionInitialized(sessionID)
		return protocol.NewSuccessResponse(*env.ID, s.initResult)
	case protocol.IsNotification(env) && isInitializedNotification(env.Method):
		s.markSessionInitialized(sessionID)
		return nil, nil
	case protocol.IsNotification(env):
		if err := s.ensureStarted(ctx); err != nil {
			return nil, err
		}
		return nil, s.process.Send(raw)
	}

	if !s.sessionInitialized(sessionID) {
		if env.ID == nil {
			return nil, nil
		}
		return protocol.NewErrorResponse(*env.ID, -32002, "session must initialize before making requests")
	}

	if err := s.ensureStarted(ctx); err != nil {
		if env.ID == nil {
			return nil, err
		}
		return protocol.NewErrorResponse(*env.ID, -32000, err.Error())
	}
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	s.mu.Lock()
	s.activeReverseSession = sessionID
	s.mu.Unlock()
	resp, err := s.engine.ForwardRequest(ctx, raw, s.process.Send)
	s.mu.Lock()
	s.activeReverseSession = ""
	s.mu.Unlock()
	return resp, err
}

func (s *sharedServer) ensureStarted(ctx context.Context) error {
	s.mu.Lock()
	if s.process != nil {
		s.mu.Unlock()
		return nil
	}
	if s.starting {
		wait := s.startWait
		s.mu.Unlock()
		select {
		case <-wait:
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.process == nil && s.lastError != "" {
				return errors.New(s.lastError)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.starting = true
	s.status = "starting"
	s.startWait = make(chan struct{})
	wait := s.startWait
	s.mu.Unlock()

	err := s.startProcess(ctx)

	s.mu.Lock()
	s.starting = false
	close(wait)
	s.mu.Unlock()
	return err
}

func (s *sharedServer) startProcess(ctx context.Context) error {
	var proc *process.ManagedProcess
	var err error

	proc, err = s.supervisor.Start(s.name, s.def, func(raw []byte) {
		reply, handlerErr := s.engine.HandleBackendMessage(raw)
		if handlerErr != nil && s.logger != nil {
			s.logger.Printf("[%s] backend message error: %v", s.name, handlerErr)
			return
		}
		if len(reply) > 0 && proc != nil {
			_ = proc.Send(reply)
		}
	})
	if err != nil {
		s.setStartError(fmt.Errorf("spawn process: %w", err))
		return err
	}

	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	initReq := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"mcp-hub","version":"0.1.0"}}}`)
	initResp, err := s.engine.ForwardRequest(initCtx, initReq, proc.Send)
	if err != nil {
		_ = proc.Close()
		s.setStartError(fmt.Errorf("initialize backend: %w", err))
		return err
	}

	if err := proc.Send([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); err != nil {
		_ = proc.Close()
		s.setStartError(fmt.Errorf("send initialized notification: %w", err))
		return err
	}

	env, err := protocol.Parse(initResp)
	if err != nil {
		_ = proc.Close()
		s.setStartError(fmt.Errorf("parse initialize response: %w", err))
		return err
	}
	if env.Error != nil {
		_ = proc.Close()
		s.setStartError(fmt.Errorf("backend initialize error: %s", env.Error.Message))
		return fmt.Errorf("backend initialize error: %s", env.Error.Message)
	}
	if env.Result == nil {
		_ = proc.Close()
		s.setStartError(errors.New("backend initialize response missing result"))
		return errors.New("backend initialize response missing result")
	}

	s.mu.Lock()
	s.process = proc
	s.initResult = protocol.CloneRaw(*env.Result)
	s.status = "running"
	s.lastError = ""
	s.lastPingAt = time.Time{}
	s.lastPingMs = 0
	s.startedAt = time.Now()
	s.mu.Unlock()

	go s.watchProcess(proc)
	go s.healthLoop(proc)

	return nil
}

func (s *sharedServer) healthLoop(proc *process.ManagedProcess) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-proc.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.process != proc || s.stopping {
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()

			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := s.engine.ForwardRequest(ctx, []byte(`{"jsonrpc":"2.0","id":999999,"method":"ping","params":{}}`), proc.Send)
			cancel()

			s.mu.Lock()
			if err != nil {
				s.lastError = fmt.Sprintf("health ping failed: %v", err)
			} else {
				s.lastPingAt = time.Now()
				s.lastPingMs = time.Since(start).Milliseconds()
			}
			s.mu.Unlock()
		}
	}
}

func (s *sharedServer) setStartError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = "error"
	s.lastError = err.Error()
}

func (s *sharedServer) watchProcess(proc *process.ManagedProcess) {
	err := <-proc.Done()
	s.mu.Lock()
	if s.process != proc {
		s.mu.Unlock()
		return
	}
	s.process = nil
	if s.stopping {
		s.status = "stopped"
		s.mu.Unlock()
		return
	}
	s.restarts++
	s.status = "restarting"
	s.lastError = fmt.Sprintf("process exited: %v", err)
	s.lastRestartAt = time.Now()
	attempt := s.restarts
	// Purge stale subscriber channels so SSE goroutines from the old process
	// drain and exit rather than accumulating alongside new subscribers (fix 0b).
	for _, sess := range s.sessions {
		for id, ch := range sess.subscribers {
			delete(sess.subscribers, id)
			close(ch)
		}
		sess.initialized = false
	}
	stopCh := s.stopRestart
	s.mu.Unlock()

	delay := time.Duration(1<<min(attempt-1, 5)) * time.Second
	// Use stopRestart channel so deleteSession/close can interrupt the backoff
	// without contending on s.mu (fix 0a).
	go func() {
		select {
		case <-time.After(delay):
		case <-stopCh:
			return
		}
		_ = s.ensureStarted(context.Background())
	}()
}

func (s *sharedServer) handleReverseRequest(raw []byte) ([]byte, error) {
	env, err := protocol.Parse(raw)
	if err != nil {
		return nil, err
	}
	if env.ID == nil {
		return nil, nil
	}
	if !isSupportedReverseMethod(env.Method) {
		return protocol.NewErrorResponse(*env.ID, -32601, "reverse method is not supported")
	}
	targetSession, err := s.resolveReverseTargetSession(raw)
	if err != nil {
		return protocol.NewErrorResponse(*env.ID, -32001, err.Error())
	}
	waitCh := make(chan []byte, 1)
	key := idKey(*env.ID)
	s.mu.Lock()
	s.reverseReq[key] = reverseWaiter{
		sessionID: targetSession,
		ch:        waitCh,
	}
	s.mu.Unlock()
	s.broadcastNotificationToSession(raw, targetSession)

	select {
	case response, ok := <-waitCh:
		if !ok {
			return protocol.NewErrorResponse(*env.ID, -32001, "session closed while waiting for client response")
		}
		return response, nil
	case <-time.After(30 * time.Second):
		s.mu.Lock()
		delete(s.reverseReq, key)
		s.mu.Unlock()
		return protocol.NewErrorResponse(*env.ID, -32001, "timed out waiting for client response")
	}
}

func (s *sharedServer) resolveReverseRequest(responseID string, raw []byte) bool {
	s.mu.Lock()
	waiter, ok := s.reverseReq[responseID]
	if ok {
		delete(s.reverseReq, responseID)
	}
	s.mu.Unlock()
	if !ok {
		return false
	}
	waiter.ch <- append([]byte(nil), raw...)
	close(waiter.ch)
	return true
}

func (s *sharedServer) markSessionInitialized(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.ensureSessionLocked(sessionID)
	session.initialized = true
}

func (s *sharedServer) sessionInitialized(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.ensureSessionLocked(sessionID)
	return session.initialized
}

func (s *sharedServer) ensureSessionLocked(sessionID string) *session {
	current, ok := s.sessions[sessionID]
	if !ok {
		current = &session{
			subscribers: map[uint64]chan []byte{},
		}
		s.sessions[sessionID] = current
	}
	return current
}

func (s *sharedServer) subscribe(sessionID string) (<-chan []byte, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.ensureSessionLocked(sessionID)
	current.nextSubID++
	id := current.nextSubID
	ch := make(chan []byte, 16)
	current.subscribers[id] = ch
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if session, ok := s.sessions[sessionID]; ok {
			if sub, found := session.subscribers[id]; found {
				delete(session.subscribers, id)
				close(sub)
			}
		}
	}
	return ch, cancel
}

func (s *sharedServer) deleteSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.sessions[sessionID]; ok {
		for id, ch := range current.subscribers {
			delete(current.subscribers, id)
			close(ch)
		}
		delete(s.sessions, sessionID)
	}
	for tok, sid := range s.progressByToken {
		if sid == sessionID {
			delete(s.progressByToken, tok)
		}
	}
	for id, waiter := range s.reverseReq {
		if waiter.sessionID == sessionID {
			delete(s.reverseReq, id)
			close(waiter.ch)
		}
	}
	// Signal any in-progress restart backoff to abort (fix 0a). Replace the
	// channel so future restarts get a fresh signal.
	close(s.stopRestart)
	s.stopRestart = make(chan struct{})
}

func (s *sharedServer) broadcastNotification(raw []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	targetSession := ""
	if tok, ok := progressTokenFromNotification(raw); ok {
		targetSession = s.progressByToken[tok]
	}
	for _, session := range s.sessions {
		if targetSession != "" && session != s.sessions[targetSession] {
			continue
		}
		for _, subscriber := range session.subscribers {
			select {
			case subscriber <- append([]byte(nil), raw...):
			default:
			}
		}
	}
}

func (s *sharedServer) broadcastNotificationToSession(raw []byte, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	for _, subscriber := range target.subscribers {
		select {
		case subscriber <- append([]byte(nil), raw...):
		default:
		}
	}
}

func (s *sharedServer) statusSnapshot() ServerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := s.status
	pid := 0
	if s.process != nil {
		pid = s.process.PID()
	}
	if status == "" {
		status = "idle"
	}
	if s.def.Mode == config.ModeShared && s.process == nil && status == "running" {
		status = "idle"
	}

	return ServerStatus{
		Name:          s.name,
		Mode:          s.def.Mode,
		Status:        status,
		PID:           pid,
		Clients:       len(s.sessions),
		Restarts:      s.restarts,
		UptimeSec:     uptimeSeconds(s.startedAt),
		LastRestartAt: formatTime(s.lastRestartAt),
		LastError:     s.lastError,
		Lazy:          s.def.IsLazy(),
		LastPingMs:    s.lastPingMs,
		LastPingAt:    formatTime(s.lastPingAt),
	}
}

func (s *sharedServer) sessionIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	return ids
}

func (s *sharedServer) close() error {
	s.mu.Lock()
	s.stopping = true
	proc := s.process
	s.process = nil
	s.startedAt = time.Time{}
	for sessionID, current := range s.sessions {
		for id, ch := range current.subscribers {
			delete(current.subscribers, id)
			close(ch)
		}
		delete(s.sessions, sessionID)
	}
	for id, waiter := range s.reverseReq {
		delete(s.reverseReq, id)
		close(waiter.ch)
	}
	clear(s.progressByToken)
	// Interrupt any pending restart backoff goroutine (fix 0a).
	close(s.stopRestart)
	s.stopRestart = make(chan struct{})
	s.mu.Unlock()

	if proc != nil {
		return proc.Close()
	}
	return nil
}

func newSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func isInitializedNotification(method string) bool {
	return method == "initialized" || method == "notifications/initialized"
}

func isSupportedReverseMethod(method string) bool {
	switch method {
	case "roots/list", "sampling/createMessage", "elicitation/create":
		return true
	default:
		return false
	}
}

func (s *sharedServer) resolveReverseTargetSession(raw []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tok, ok := progressTokenFromRequest(raw); ok {
		if sid, found := s.progressByToken[tok]; found {
			if sess, ok := s.sessions[sid]; ok && sess.initialized {
				return sid, nil
			}
		}
	}

	if sid := s.activeReverseSession; sid != "" {
		if sess, ok := s.sessions[sid]; ok && sess.initialized {
			return sid, nil
		}
	}

	initialized := make([]string, 0, len(s.sessions))
	for sid, sess := range s.sessions {
		if sess.initialized {
			initialized = append(initialized, sid)
		}
	}
	switch len(initialized) {
	case 0:
		return "", errors.New("no initialized client session available for reverse request")
	case 1:
		return initialized[0], nil
	default:
		return "", errors.New("multiple initialized sessions available; reverse request target is ambiguous")
	}
}

func progressTokenFromRequest(raw []byte) (string, bool) {
	var msg map[string]any
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", false
	}
	params, ok := msg["params"].(map[string]any)
	if !ok {
		return "", false
	}
	if tok, ok := params["progressToken"]; ok {
		return fmt.Sprint(tok), true
	}
	meta, ok := params["_meta"].(map[string]any)
	if !ok {
		return "", false
	}
	if tok, ok := meta["progressToken"]; ok {
		return fmt.Sprint(tok), true
	}
	return "", false
}

func progressTokenFromNotification(raw []byte) (string, bool) {
	env, err := protocol.Parse(raw)
	if err != nil || env.Method != "notifications/progress" {
		return "", false
	}
	var msg map[string]any
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", false
	}
	params, ok := msg["params"].(map[string]any)
	if !ok {
		return "", false
	}
	if tok, ok := params["progressToken"]; ok {
		return fmt.Sprint(tok), true
	}
	if tok, ok := params["token"]; ok {
		return fmt.Sprint(tok), true
	}
	return "", false
}

func mergeInitializeResult(merged map[string]any, rawResponse []byte) map[string]any {
	if len(rawResponse) == 0 {
		return merged
	}
	env, err := protocol.Parse(rawResponse)
	if err != nil || env.Result == nil {
		return merged
	}
	var result map[string]any
	if err := json.Unmarshal(*env.Result, &result); err != nil {
		return merged
	}
	if merged == nil {
		merged = map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"serverInfo": map[string]any{
				"name":    "mcp-hub",
				"version": Version,
			},
		}
	}
	if pv, ok := result["protocolVersion"].(string); ok && pv != "" {
		merged["protocolVersion"] = pv
	}
	dstCaps, _ := merged["capabilities"].(map[string]any)
	srcCaps, _ := result["capabilities"].(map[string]any)
	if dstCaps == nil {
		dstCaps = map[string]any{}
		merged["capabilities"] = dstCaps
	}
	for k, v := range srcCaps {
		if existing, ok := dstCaps[k].(map[string]any); ok {
			if incoming, ok := v.(map[string]any); ok {
				for kk, vv := range incoming {
					existing[kk] = vv
				}
				dstCaps[k] = existing
				continue
			}
		}
		dstCaps[k] = v
	}
	return merged
}

func validateAggregateInitializeResponse(serverName string, rawResponse []byte) error {
	if len(rawResponse) == 0 {
		return fmt.Errorf("initialize %s: backend returned no response", serverName)
	}
	env, err := protocol.Parse(rawResponse)
	if err != nil {
		return fmt.Errorf("initialize %s: parse backend response: %w", serverName, err)
	}
	if env.Error != nil {
		return fmt.Errorf("initialize %s: %s", serverName, env.Error.Message)
	}
	if env.Result == nil {
		return fmt.Errorf("initialize %s: backend initialize response missing result", serverName)
	}
	return nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func uptimeSeconds(start time.Time) int64 {
	if start.IsZero() {
		return 0
	}
	sec := int64(time.Since(start).Seconds())
	if sec < 0 {
		return 0
	}
	return sec
}
