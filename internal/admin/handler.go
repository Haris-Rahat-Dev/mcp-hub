package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
	"github.com/harisbinrahat/mcp-hub/internal/hub"
	"github.com/harisbinrahat/mcp-hub/internal/importer"
	"github.com/harisbinrahat/mcp-hub/internal/launch"
	"gopkg.in/yaml.v3"
)

// Handler serves the JSON admin API for the control panel.
type Handler struct {
	Hub     *hub.Hub
	CfgPath string
	Logger  *log.Logger
}

// Mount registers /api/v1/* routes on mux. All routes require the same auth as MCP
// (Bearer token when hub API key is set). When the API key is empty, only loopback
// clients may access the admin API.
func Mount(mux *http.ServeMux, h *hub.Hub, cfgPath string, logger *log.Logger) {
	a := &Handler{Hub: h, CfgPath: cfgPath, Logger: logger}
	wrap := func(next http.HandlerFunc) http.Handler {
		return a.withAuth(http.HandlerFunc(next))
	}

	mux.Handle("GET /api/v1/meta", wrap(a.handleMeta))
	mux.Handle("GET /api/v1/status", wrap(a.handleStatus))
	mux.Handle("GET /api/v1/config", wrap(a.handleGetConfig))
	mux.Handle("PUT /api/v1/config", wrap(a.handlePutConfig))
	mux.Handle("GET /api/v1/servers", wrap(a.handleListServers))
	mux.Handle("POST /api/v1/servers", wrap(a.handleAddServer))
	mux.Handle("DELETE /api/v1/servers/{name}", wrap(a.handleRemoveServer))
	mux.Handle("GET /api/v1/clients", wrap(a.handleListClients))
	mux.Handle("POST /api/v1/clients", wrap(a.handleAddClient))
	mux.Handle("DELETE /api/v1/clients/{id}", wrap(a.handleRemoveClient))
	mux.Handle("POST /api/v1/sync", wrap(a.handleSync))
	mux.Handle("POST /api/v1/import", wrap(a.handleImport))
	mux.Handle("POST /api/v1/launch", wrap(a.handleLaunch))
}

func (a *Handler) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key := a.Hub.APIKey(); key != "" {
			if r.Header.Get("Authorization") != "Bearer "+key {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		} else {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil || (host != "127.0.0.1" && host != "::1") {
				http.Error(w, "admin API only from localhost when hub.api_key is unset", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (a *Handler) handleMeta(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"version":        hub.Version,
		"requires_auth":  a.Hub.APIKey() != "",
		"config_path":    a.CfgPath,
		"listen_address": a.Hub.Status().Address,
	})
}

func (a *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.Hub.Status())
}

func (a *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path, err := config.ResolveConfigPath(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			_ = json.NewEncoder(w).Encode(map[string]string{"yaml": ""})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"yaml": string(data)})
}

func (a *Handler) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(body.YAML), &cfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid yaml: %v", err), http.StatusBadRequest)
		return
	}
	path, err := config.ResolveConfigPath(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.Path = path
	if err := cfg.PrepareAndValidate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := config.Save(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reloaded, err := config.Load(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.Hub.Reload(context.Background(), reloaded); err != nil {
		http.Error(w, fmt.Sprintf("saved config but reload failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Handler) handleListServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg, err := config.LoadForMutation(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	names := make([]string, 0, len(cfg.Servers))
	for n := range cfg.Servers {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		s := cfg.Servers[name]
		target := s.Command
		if s.URL != "" {
			target = s.URL
		}
		out = append(out, map[string]any{
			"name":          name,
			"mode":          string(s.Mode),
			"command":       s.Command,
			"args":          s.Args,
			"url":           s.URL,
			"env":           s.Env,
			"headers":       s.Headers,
			"lazy":          s.IsLazy(),
			"max_instances": s.MaxInstances,
			"target":        target,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"servers": out})
}

func (a *Handler) handleAddServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string            `json:"name"`
		Mode         string            `json:"mode"`
		Command      string            `json:"command"`
		Args         []string          `json:"args"`
		URL          string            `json:"url"`
		Env          map[string]string `json:"env"`
		Headers      map[string]string `json:"headers"`
		Lazy         *bool             `json:"lazy"`
		MaxInstances int               `json:"max_instances"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	cfg, err := config.LoadForMutation(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	m := config.Mode(body.Mode)
	if m == "" {
		if body.URL != "" {
			m = config.ModePassthrough
		} else {
			m = config.ModeShared
		}
	}
	if body.Lazy == nil && m != config.ModePassthrough {
		// If unspecified, default to lazy-start (shared/isolated) to match ServerConfig.IsLazy behavior.
		v := true
		body.Lazy = &v
	}
	if body.Env == nil {
		body.Env = map[string]string{}
	}
	if body.Headers == nil {
		body.Headers = map[string]string{}
	}
	server := config.ServerConfig{
		Command:      body.Command,
		Args:         body.Args,
		URL:          body.URL,
		Mode:         m,
		Lazy:         body.Lazy,
		Env:          body.Env,
		Headers:      body.Headers,
		MaxInstances: body.MaxInstances,
	}
	switch server.Mode {
	case config.ModeShared, config.ModeIsolated:
		if server.Command == "" {
			http.Error(w, fmt.Sprintf("--command is required for %s mode", server.Mode), http.StatusBadRequest)
			return
		}
		server.URL = ""
	case config.ModePassthrough:
		if server.URL == "" {
			http.Error(w, "url is required for passthrough mode", http.StatusBadRequest)
			return
		}
		server.Command = ""
		server.Args = nil
		server.Env = nil
	default:
		http.Error(w, "unsupported mode", http.StatusBadRequest)
		return
	}
	cfg.Servers[body.Name] = server
	if err := config.Save(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reloaded, err := config.Load(cfg.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.Hub.Reload(context.Background(), reloaded); err != nil {
		http.Error(w, fmt.Sprintf("saved but reload failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *Handler) handleRemoveServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}
	cfg, err := config.LoadForMutation(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := cfg.Servers[name]; !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	delete(cfg.Servers, name)
	if err := config.Save(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reloaded, err := config.Load(cfg.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.Hub.Reload(context.Background(), reloaded); err != nil {
		http.Error(w, fmt.Sprintf("saved but reload failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Handler) handleListClients(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg, err := config.Load(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	clients := generate.ResolveClients(cfg)
	_ = json.NewEncoder(w).Encode(map[string]any{"clients": clients})
}

func (a *Handler) handleAddClient(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
		config.ClientManifest
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.ID) == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	cfg, err := config.LoadForMutation(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if cfg.Clients == nil {
		cfg.Clients = map[string]config.ClientManifest{}
	}
	cfg.Clients[body.ID] = body.ClientManifest
	if err := config.Save(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *Handler) handleRemoveClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	cfg, err := config.LoadForMutation(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, ok := cfg.Clients[id]; !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	delete(cfg.Clients, id)
	if err := config.Save(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Handler) handleSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		ClientID string `json:"client_id"`
		DryRun   bool   `json:"dry_run"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&body); err != nil && err != io.EOF {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := config.Load(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if body.ClientID != "" {
		res, err := generate.Sync(cfg, body.ClientID, body.DryRun)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []generate.SyncResult{res}})
		return
	}
	results, err := generate.SyncAll(cfg, body.DryRun)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func (a *Handler) handleImport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		JSON     string `json:"json"`
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.JSON == "" {
		http.Error(w, "json is required", http.StatusBadRequest)
		return
	}
	n, err := importer.ImportMCPJSON(a.CfgPath, []byte(body.JSON), body.ClientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path, err := config.ResolveConfigPath(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	full, err := config.Load(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.Hub.Reload(context.Background(), full); err != nil {
		http.Error(w, fmt.Sprintf("imported but reload failed: %v", err), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"imported": n})
}

func (a *Handler) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ClientID == "" {
		http.Error(w, "client_id is required", http.StatusBadRequest)
		return
	}
	cfg, err := config.Load(a.CfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := generate.Sync(cfg, body.ClientID, false); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	clients := generate.ResolveClients(cfg)
	manifest, ok := clients[body.ClientID]
	if !ok {
		http.Error(w, "unknown client", http.StatusNotFound)
		return
	}
	if err := launch.Manifest(manifest); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
