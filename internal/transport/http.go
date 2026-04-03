package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/harisbinrahat/mcp-hub/internal/hub"
)

type Handler struct {
	hub    *hub.Hub
	logger *log.Logger
}

func NewHandler(h *hub.Hub, logger *log.Logger) *Handler {
	return &Handler{hub: h, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.hub.APIKey() != "" {
		expected := "Bearer " + h.hub.APIKey()
		if r.Header.Get("Authorization") != expected {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Client-scoped routes: /clients/<client-id>/mcp and /clients/<client-id>/mcp/<server>
	if strings.HasPrefix(r.URL.Path, "/clients/") {
		h.handleClientScoped(w, r)
		return
	}

	switch {
	case r.URL.Path == "/status" && r.Method == http.MethodGet:
		h.handleStatus(w, r)
	case r.URL.Path == "/mcp":
		h.handleAggregate(w, r)
	case strings.HasPrefix(r.URL.Path, "/mcp/"):
		h.handleMCP(w, r, "unknown")
	default:
		http.NotFound(w, r)
	}
}

// handleClientScoped handles /clients/<client-id>/mcp and /clients/<client-id>/mcp/<server>.
func (h *Handler) handleClientScoped(w http.ResponseWriter, r *http.Request) {
	// Strip /clients/ prefix and split into clientID + rest.
	rest := strings.TrimPrefix(r.URL.Path, "/clients/")
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		http.NotFound(w, r)
		return
	}
	clientID := rest[:slash]
	remainder := rest[slash+1:] // e.g. "mcp" or "mcp/<server>"

	if clientID == "" {
		http.NotFound(w, r)
		return
	}

	switch {
	case remainder == "mcp":
		h.handleAggregateWithClient(w, r, clientID)
	case strings.HasPrefix(remainder, "mcp/"):
		serverName := strings.TrimPrefix(remainder, "mcp/")
		if serverName == "" || strings.Contains(serverName, "/") {
			http.NotFound(w, r)
			return
		}
		h.handleMCP(w, r, clientID)
	default:
		http.NotFound(w, r)
	}
}

// handleAggregateWithClient is like handleAggregate but records the client id.
func (h *Handler) handleAggregateWithClient(w http.ResponseWriter, r *http.Request, clientID string) {
	// For now, aggregate with client id uses the same underlying aggregate logic.
	// Client tracking is recorded via the X-MCP-Client-ID header path.
	r.Header.Set("X-MCP-Client-ID", clientID)
	h.handleAggregate(w, r)
}

func (h *Handler) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.hub.Status())
}

func (h *Handler) handleMCP(w http.ResponseWriter, r *http.Request, clientID string) {
	// Extract server name from either /mcp/<server> or /clients/<id>/mcp/<server>.
	var serverName string
	if strings.HasPrefix(r.URL.Path, "/clients/") {
		// /clients/<id>/mcp/<server>
		rest := strings.TrimPrefix(r.URL.Path, "/clients/")
		idx := strings.Index(rest, "/mcp/")
		if idx < 0 {
			http.NotFound(w, r)
			return
		}
		serverName = rest[idx+5:]
	} else {
		serverName = strings.TrimPrefix(r.URL.Path, "/mcp/")
	}
	if serverName == "" || strings.Contains(serverName, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleStream(w, r, serverName, clientID)
	case http.MethodPost:
		h.handlePost(w, r, serverName, clientID)
	case http.MethodDelete:
		h.handleDelete(w, r, serverName)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleAggregate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleAggregateStream(w, r)
	case http.MethodPost:
		h.handleAggregatePost(w, r)
	case http.MethodDelete:
		h.handleAggregateDelete(w, r)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request, serverName, clientID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	sessionID, ch, cancel, err := h.hub.Subscribe(serverName, r.Header.Get("Mcp-Session-Id"), clientID)
	if err != nil {
		h.writeHubError(w, err)
		return
	}
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Mcp-Session-Id", sessionID)

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request, serverName, clientID string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	sessionID, response, err := h.hub.HandleRequest(r.Context(), serverName, r.Header.Get("Mcp-Session-Id"), clientID, body)
	if err != nil {
		h.writeHubError(w, err)
		return
	}

	w.Header().Set("Mcp-Session-Id", sessionID)
	if len(response) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(response)
}

func (h *Handler) handleAggregatePost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	sessionID, response, err := h.hub.HandleAggregateRequest(r.Context(), r.Header.Get("Mcp-Session-Id"), body)
	if err != nil {
		h.writeHubError(w, err)
		return
	}

	w.Header().Set("Mcp-Session-Id", sessionID)
	if len(response) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(response)
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request, serverName string) {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		http.Error(w, "Mcp-Session-Id header is required", http.StatusBadRequest)
		return
	}
	if err := h.hub.DeleteSession(serverName, sessionID); err != nil {
		h.writeHubError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleAggregateDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		http.Error(w, "Mcp-Session-Id header is required", http.StatusBadRequest)
		return
	}
	if err := h.hub.DeleteAggregateSession(sessionID); err != nil {
		h.writeHubError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleAggregateStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	sessionID, ch, cancel, err := h.hub.SubscribeAggregate(r.Header.Get("Mcp-Session-Id"))
	if err != nil {
		h.writeHubError(w, err)
		return
	}
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Mcp-Session-Id", sessionID)

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) writeHubError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hub.ErrUnknownServer):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, hub.ErrModeNotImplemented):
		http.Error(w, err.Error(), http.StatusNotImplemented)
	default:
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}
