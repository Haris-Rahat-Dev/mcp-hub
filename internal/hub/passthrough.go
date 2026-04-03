package hub

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
)

type passthroughSession struct {
	nextSubID         uint64
	subscribers       map[uint64]chan []byte
	upstreamSessionID string
	streamCancel      context.CancelFunc
	streamRunning     bool
}

type passthroughServer struct {
	name   string
	def    config.ServerConfig
	logger *log.Logger
	client *http.Client
	streamClient *http.Client

	mu       sync.Mutex
	sessions map[string]*passthroughSession
}

func newPassthroughServer(name string, def config.ServerConfig, logger *log.Logger) *passthroughServer {
	return &passthroughServer{
		name:     name,
		def:      def,
		logger:   logger,
		client:   &http.Client{Timeout: 30 * time.Second},
		streamClient: &http.Client{},
		sessions: map[string]*passthroughSession{},
	}
}

func (s *passthroughServer) ensureSessionID(existing string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing == "" {
		existing = newSessionID()
	}
	s.ensureSessionLocked(existing)
	return existing
}

func (s *passthroughServer) handleRequest(ctx context.Context, sessionID string, raw []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.def.URL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	s.applyForwardHeaders(req)

	upstreamSessionID := s.upstreamSessionID(sessionID)
	if upstreamSessionID != "" {
		req.Header.Set("Mcp-Session-Id", upstreamSessionID)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if headerSID := resp.Header.Get("Mcp-Session-Id"); headerSID != "" {
		s.setUpstreamSessionID(sessionID, headerSID)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("passthrough request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (s *passthroughServer) subscribe(sessionID string) (<-chan []byte, func()) {
	s.mu.Lock()
	current := s.ensureSessionLocked(sessionID)
	current.nextSubID++
	id := current.nextSubID
	ch := make(chan []byte, 16)
	current.subscribers[id] = ch
	if !current.streamRunning {
		streamCtx, cancel := context.WithCancel(context.Background())
		current.streamCancel = cancel
		current.streamRunning = true
		go s.streamSession(streamCtx, sessionID)
	}
	s.mu.Unlock()

	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		current, ok := s.sessions[sessionID]
		if !ok {
			return
		}
		if sub, found := current.subscribers[id]; found {
			delete(current.subscribers, id)
			close(sub)
		}
		if len(current.subscribers) == 0 && current.streamCancel != nil {
			current.streamCancel()
			current.streamCancel = nil
			current.streamRunning = false
		}
	}
	return ch, cancel
}

func (s *passthroughServer) deleteSession(sessionID string) {
	s.mu.Lock()
	current, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return
	}
	upstreamSessionID := current.upstreamSessionID
	if current.streamCancel != nil {
		current.streamCancel()
	}
	for id, ch := range current.subscribers {
		delete(current.subscribers, id)
		close(ch)
	}
	delete(s.sessions, sessionID)
	s.mu.Unlock()

	if upstreamSessionID != "" {
		req, err := http.NewRequest(http.MethodDelete, s.def.URL, nil)
		if err != nil {
			return
		}
		s.applyForwardHeaders(req)
		req.Header.Set("Mcp-Session-Id", upstreamSessionID)
		resp, err := s.client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}
}

func (s *passthroughServer) statusSnapshot() ServerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := "idle"
	if len(s.sessions) > 0 {
		status = "running"
	}
	return ServerStatus{
		Name:      s.name,
		Mode:      s.def.Mode,
		Status:    status,
		PID:       0,
		Clients:   len(s.sessions),
		Restarts:  0,
		LastError: "",
		Lazy:      s.def.IsLazy(),
	}
}

func (s *passthroughServer) close() error {
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

func (s *passthroughServer) streamSession(ctx context.Context, sessionID string) {
	defer s.markStreamStopped(sessionID)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.def.URL, nil)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		s.applyForwardHeaders(req)
		if upstream := s.upstreamSessionID(sessionID); upstream != "" {
			req.Header.Set("Mcp-Session-Id", upstream)
		}

		resp, err := s.streamClient.Do(req)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if headerSID := resp.Header.Get("Mcp-Session-Id"); headerSID != "" {
			s.setUpstreamSessionID(sessionID, headerSID)
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 2<<20)
		var dataLines []string
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				_ = resp.Body.Close()
				return
			default:
			}
			line := scanner.Text()
			if line == "" {
				if len(dataLines) > 0 {
					payload := strings.Join(dataLines, "\n")
					s.broadcastNotification(sessionID, []byte(payload))
					dataLines = dataLines[:0]
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		_ = resp.Body.Close()
		time.Sleep(250 * time.Millisecond)
	}
}

func (s *passthroughServer) broadcastNotification(sessionID string, raw []byte) {
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

func (s *passthroughServer) upstreamSessionID(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[sessionID]
	if !ok {
		return ""
	}
	return current.upstreamSessionID
}

func (s *passthroughServer) setUpstreamSessionID(sessionID, upstreamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.ensureSessionLocked(sessionID)
	current.upstreamSessionID = upstreamID
}

func (s *passthroughServer) markStreamStopped(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	current.streamRunning = false
	current.streamCancel = nil
}

func (s *passthroughServer) applyForwardHeaders(req *http.Request) {
	for key, value := range s.def.Headers {
		req.Header.Set(key, value)
	}
}

func (s *passthroughServer) ensureSessionLocked(sessionID string) *passthroughSession {
	current, ok := s.sessions[sessionID]
	if !ok {
		current = &passthroughSession{
			subscribers: map[uint64]chan []byte{},
		}
		s.sessions[sessionID] = current
	}
	return current
}

func (s *passthroughServer) sessionIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	return ids
}

var _ serverRuntime = (*passthroughServer)(nil)

