package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/multiplex"
	"github.com/harisbinrahat/mcp-hub/internal/process"
	"github.com/harisbinrahat/mcp-hub/internal/protocol"
)

type isolatedSession struct {
	initialized bool
	nextSubID   uint64
	subscribers map[uint64]chan []byte
	reverseReq  map[string]chan []byte

	process    *process.ManagedProcess
	engine     *multiplex.Engine
	initResult json.RawMessage

	starting   bool
	startWait  chan struct{}
	status     string
	lastError  string
	restarts   int
	stopping   bool
	lastPingMs int64
	lastPingAt time.Time

	// ctx/cancel allow reverse-request waiters to unblock when the session
	// is torn down, preventing goroutine leaks (fix 0d).
	ctx    context.Context
	cancel context.CancelFunc
}

type isolatedServer struct {
	name       string
	def        config.ServerConfig
	logger     *log.Logger
	supervisor *process.Supervisor

	mu       sync.Mutex
	sessions map[string]*isolatedSession
}

func newIsolatedServer(name string, def config.ServerConfig, logger *log.Logger) *isolatedServer {
	return &isolatedServer{
		name:       name,
		def:        def,
		logger:     logger,
		supervisor: &process.Supervisor{Logger: logger},
		sessions:   map[string]*isolatedSession{},
	}
}

func (s *isolatedServer) ensureSessionID(existing string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing == "" {
		existing = newSessionID()
	}
	s.ensureSessionLocked(existing)
	return existing
}

func (s *isolatedServer) handleRequest(ctx context.Context, sessionID string, raw []byte) ([]byte, error) {
	if s.def.Mode != config.ModeIsolated {
		return nil, ErrModeNotImplemented
	}

	env, err := protocol.Parse(raw)
	if err != nil {
		return nil, err
	}
	if protocol.IsResponse(env) && env.ID != nil {
		if s.resolveReverseRequest(sessionID, idKey(*env.ID), raw) {
			return nil, nil
		}
	}

	switch {
	case protocol.IsRequest(env) && env.Method == "initialize":
		if err := s.ensureStarted(ctx, sessionID); err != nil {
			return protocol.NewErrorResponse(*env.ID, -32000, err.Error())
		}
		s.markSessionInitialized(sessionID)
		return protocol.NewSuccessResponse(*env.ID, s.sessionInitResult(sessionID))
	case protocol.IsNotification(env) && isInitializedNotification(env.Method):
		s.markSessionInitialized(sessionID)
		return nil, nil
	case protocol.IsNotification(env):
		if err := s.ensureStarted(ctx, sessionID); err != nil {
			return nil, err
		}
		return nil, s.sessionProcessSend(sessionID, raw)
	}

	if !s.sessionInitialized(sessionID) {
		if env.ID == nil {
			return nil, nil
		}
		return protocol.NewErrorResponse(*env.ID, -32002, "session must initialize before making requests")
	}

	if err := s.ensureStarted(ctx, sessionID); err != nil {
		if env.ID == nil {
			return nil, err
		}
		return protocol.NewErrorResponse(*env.ID, -32000, err.Error())
	}

	engine, send := s.sessionEngineAndSend(sessionID)
	return engine.ForwardRequest(ctx, raw, send)
}

func (s *isolatedServer) ensureStarted(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	current := s.ensureSessionLocked(sessionID)
	if current.process != nil {
		s.mu.Unlock()
		return nil
	}
	if current.starting {
		wait := current.startWait
		s.mu.Unlock()
		select {
		case <-wait:
			s.mu.Lock()
			defer s.mu.Unlock()
			current = s.ensureSessionLocked(sessionID)
			if current.process == nil && current.lastError != "" {
				return errors.New(current.lastError)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if s.def.MaxInstances > 0 {
		activeCount := 0
		for _, sess := range s.sessions {
			if sess.process != nil || sess.starting {
				activeCount++
			}
		}
		if activeCount >= s.def.MaxInstances {
			s.mu.Unlock()
			return fmt.Errorf("server %q has reached max instances (%d)", s.name, s.def.MaxInstances)
		}
	}

	current.starting = true
	current.status = "starting"
	current.startWait = make(chan struct{})
	wait := current.startWait
	s.mu.Unlock()

	err := s.startSessionProcess(ctx, sessionID)

	s.mu.Lock()
	current = s.ensureSessionLocked(sessionID)
	current.starting = false
	close(wait)
	s.mu.Unlock()
	return err
}

func (s *isolatedServer) startSessionProcess(ctx context.Context, sessionID string) error {
	var proc *process.ManagedProcess

	onNotification := func(raw []byte) {
		s.broadcastNotification(sessionID, raw)
	}
	onReverse := func(raw []byte) ([]byte, error) {
		return s.handleReverseRequest(sessionID, raw)
	}
	engine := multiplex.NewEngine(onNotification, onReverse)

	var err error
	proc, err = s.supervisor.Start(s.name, s.def, func(raw []byte) {
		reply, handlerErr := engine.HandleBackendMessage(raw)
		if handlerErr != nil && s.logger != nil {
			s.logger.Printf("[%s/%s] backend message error: %v", s.name, sessionID, handlerErr)
			return
		}
		if len(reply) > 0 && proc != nil {
			_ = proc.Send(reply)
		}
	})
	if err != nil {
		s.setStartError(sessionID, fmt.Errorf("spawn process: %w", err))
		return err
	}

	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	initReq := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"mcp-hub","version":"0.1.0"}}}`)
	initResp, err := engine.ForwardRequest(initCtx, initReq, proc.Send)
	if err != nil {
		_ = proc.Close()
		s.setStartError(sessionID, fmt.Errorf("initialize backend: %w", err))
		return err
	}

	if err := proc.Send([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); err != nil {
		_ = proc.Close()
		s.setStartError(sessionID, fmt.Errorf("send initialized notification: %w", err))
		return err
	}

	env, err := protocol.Parse(initResp)
	if err != nil {
		_ = proc.Close()
		s.setStartError(sessionID, fmt.Errorf("parse initialize response: %w", err))
		return err
	}
	if env.Error != nil {
		_ = proc.Close()
		s.setStartError(sessionID, fmt.Errorf("backend initialize error: %s", env.Error.Message))
		return fmt.Errorf("backend initialize error: %s", env.Error.Message)
	}
	if env.Result == nil {
		_ = proc.Close()
		s.setStartError(sessionID, errors.New("backend initialize response missing result"))
		return errors.New("backend initialize response missing result")
	}

	s.mu.Lock()
	current := s.ensureSessionLocked(sessionID)
	if current.stopping {
		s.mu.Unlock()
		_ = proc.Close()
		return errors.New("session is stopping")
	}
	current.process = proc
	current.engine = engine
	current.initResult = protocol.CloneRaw(*env.Result)
	current.status = "running"
	current.lastError = ""
	current.lastPingAt = time.Time{}
	current.lastPingMs = 0
	s.mu.Unlock()

	go s.watchProcess(sessionID, proc)
	go s.healthLoop(sessionID, proc, engine)

	return nil
}

func (s *isolatedServer) healthLoop(sessionID string, proc *process.ManagedProcess, engine *multiplex.Engine) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-proc.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			current, ok := s.sessions[sessionID]
			if !ok || current.process != proc || current.stopping {
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()

			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := engine.ForwardRequest(ctx, []byte(`{"jsonrpc":"2.0","id":999999,"method":"ping","params":{}}`), proc.Send)
			cancel()

			s.mu.Lock()
			current, ok = s.sessions[sessionID]
			if ok {
				if err != nil {
					current.lastError = fmt.Sprintf("health ping failed: %v", err)
				} else {
					current.lastPingAt = time.Now()
					current.lastPingMs = time.Since(start).Milliseconds()
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *isolatedServer) watchProcess(sessionID string, proc *process.ManagedProcess) {
	err := <-proc.Done()

	s.mu.Lock()
	current, ok := s.sessions[sessionID]
	if !ok || current.process != proc {
		s.mu.Unlock()
		return
	}
	current.process = nil
	current.engine = nil
	if current.stopping {
		current.status = "stopped"
		s.mu.Unlock()
		return
	}
	current.restarts++
	current.status = "restarting"
	current.lastError = fmt.Sprintf("process exited: %v", err)
	attempt := current.restarts
	s.mu.Unlock()

	delay := time.Duration(1<<min(attempt-1, 5)) * time.Second
	go func() {
		time.Sleep(delay)
		_ = s.ensureStarted(context.Background(), sessionID)
	}()
}

func (s *isolatedServer) setStartError(sessionID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.ensureSessionLocked(sessionID)
	current.status = "error"
	current.lastError = err.Error()
}

func (s *isolatedServer) markSessionInitialized(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.ensureSessionLocked(sessionID)
	current.initialized = true
}

func (s *isolatedServer) handleReverseRequest(sessionID string, raw []byte) ([]byte, error) {
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

	waitCh := make(chan []byte, 1)
	key := idKey(*env.ID)

	s.mu.Lock()
	current := s.ensureSessionLocked(sessionID)
	current.reverseReq[key] = waitCh
	// Capture the session context so we can unblock if the session is deleted
	// while we are waiting for the client response (fix 0d).
	sessionCtx := current.ctx
	s.mu.Unlock()

	s.broadcastNotification(sessionID, raw)

	select {
	case response, ok := <-waitCh:
		if !ok {
			return protocol.NewErrorResponse(*env.ID, -32001, "session closed while waiting for client response")
		}
		return response, nil
	case <-time.After(30 * time.Second):
		s.mu.Lock()
		if current, found := s.sessions[sessionID]; found {
			delete(current.reverseReq, key)
		}
		s.mu.Unlock()
		return protocol.NewErrorResponse(*env.ID, -32001, "timed out waiting for client response")
	case <-sessionCtx.Done():
		return protocol.NewErrorResponse(*env.ID, -32001, "session deleted while waiting for client response")
	}
}

func (s *isolatedServer) resolveReverseRequest(sessionID string, responseID string, raw []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[sessionID]
	if !ok {
		return false
	}
	ch, found := current.reverseReq[responseID]
	if !found {
		return false
	}
	delete(current.reverseReq, responseID)
	ch <- append([]byte(nil), raw...)
	close(ch)
	return true
}

func (s *isolatedServer) sessionInitialized(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.ensureSessionLocked(sessionID)
	return current.initialized
}

func (s *isolatedServer) sessionInitResult(sessionID string) json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.ensureSessionLocked(sessionID)
	return protocol.CloneRaw(current.initResult)
}

func (s *isolatedServer) sessionProcessSend(sessionID string, raw []byte) error {
	s.mu.Lock()
	current := s.ensureSessionLocked(sessionID)
	proc := current.process
	s.mu.Unlock()
	if proc == nil {
		return errors.New("backend process not running")
	}
	return proc.Send(raw)
}

func (s *isolatedServer) sessionEngineAndSend(sessionID string) (*multiplex.Engine, func([]byte) error) {
	s.mu.Lock()
	current := s.ensureSessionLocked(sessionID)
	engine := current.engine
	proc := current.process
	s.mu.Unlock()
	send := func(raw []byte) error {
		if proc == nil {
			return errors.New("backend process not running")
		}
		return proc.Send(raw)
	}
	return engine, send
}

func (s *isolatedServer) subscribe(sessionID string) (<-chan []byte, func()) {
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

func (s *isolatedServer) deleteSession(sessionID string) {
	s.mu.Lock()
	current, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return
	}
	current.stopping = true
	proc := current.process
	current.process = nil
	current.engine = nil
	for id, ch := range current.subscribers {
		delete(current.subscribers, id)
		close(ch)
	}
	for id, ch := range current.reverseReq {
		delete(current.reverseReq, id)
		close(ch)
	}
	// Cancel the session context to unblock any reverse-request waiters before
	// we remove the session entry (fix 0d).
	if current.cancel != nil {
		current.cancel()
	}
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	if proc != nil {
		_ = proc.Close()
	}
}

func (s *isolatedServer) broadcastNotification(sessionID string, raw []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	for _, subscriber := range current.subscribers {
		select {
		case subscriber <- append([]byte(nil), raw...):
		default:
		}
	}
}

func (s *isolatedServer) statusSnapshot() ServerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	clients := len(s.sessions)
	status := "idle"
	lastError := ""
	restarts := 0
	lastPingMs := int64(0)
	lastPingAt := time.Time{}

	for _, sess := range s.sessions {
		if sess.process != nil {
			status = "running"
		}
		if sess.status != "" && status != "running" {
			status = sess.status
		}
		if sess.lastError != "" {
			lastError = sess.lastError
		}
		restarts += sess.restarts
		if sess.lastPingAt.After(lastPingAt) {
			lastPingAt = sess.lastPingAt
			lastPingMs = sess.lastPingMs
		}
	}

	return ServerStatus{
		Name:      s.name,
		Mode:      s.def.Mode,
		Status:    status,
		PID:       0,
		Clients:   clients,
		Restarts:  restarts,
		LastError: lastError,
		Lazy:      s.def.IsLazy(),
		LastPingMs: lastPingMs,
		LastPingAt: formatTime(lastPingAt),
	}
}

func (s *isolatedServer) close() error {
	s.mu.Lock()
	sessionIDs := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		sessionIDs = append(sessionIDs, id)
	}
	s.mu.Unlock()

	for _, id := range sessionIDs {
		s.deleteSession(id)
	}
	return nil
}

func (s *isolatedServer) ensureSessionLocked(sessionID string) *isolatedSession {
	current, ok := s.sessions[sessionID]
	if !ok {
		ctx, cancel := context.WithCancel(context.Background())
		current = &isolatedSession{
			subscribers: map[uint64]chan []byte{},
			reverseReq:  map[string]chan []byte{},
			status:      "idle",
			ctx:         ctx,
			cancel:      cancel,
		}
		s.sessions[sessionID] = current
	}
	return current
}

func (s *isolatedServer) sessionIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	return ids
}

var _ serverRuntime = (*isolatedServer)(nil)

func idKey(raw json.RawMessage) string {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return string(raw)
	}
	return string(normalized)
}

