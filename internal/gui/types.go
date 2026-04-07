package gui

import "time"

type ServerStatus string

const (
	StatusRunning ServerStatus = "running"
	StatusIdle    ServerStatus = "idle"
	StatusError   ServerStatus = "error"
	StatusStopped ServerStatus = "stopped"
)

type ServerMode string

const (
	ModeShared      ServerMode = "shared"
	ModeIsolated    ServerMode = "isolated"
	ModePassthrough ServerMode = "passthrough"
)

type ServerInfo struct {
	Name        string       `json:"name"`
	Mode        ServerMode   `json:"mode"`
	Status      ServerStatus `json:"status"`
	Command     string       `json:"command,omitempty"`
	Args        []string     `json:"args,omitempty"`
	URL         string       `json:"url,omitempty"`
	Lazy        bool         `json:"lazy"`
	PID         int          `json:"pid,omitempty"`
	RAMMB       int          `json:"ramMB,omitempty"`
	ClientCount int          `json:"clientCount"`
	Uptime      string       `json:"uptime,omitempty"`
	LastError   string       `json:"lastError,omitempty"`
}

type ClientInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Kind        string `json:"kind"`
	ConfigPath  string `json:"configPath"`
	SyncedAt    string `json:"syncedAt,omitempty"`
	ServerCount int    `json:"serverCount"`
	LastError   string `json:"lastError,omitempty"`
}

type HubStatus struct {
	Version      string       `json:"version"`
	ListenAddr   string       `json:"listenAddress"`
	Uptime       string       `json:"uptime"`
	TotalRAM     string       `json:"totalRAM"`
	RequiresAuth bool         `json:"requiresAuth"`
	ServerCount  StatsCount   `json:"serverCount"`
	Servers      []ServerInfo `json:"servers"`
	Clients      []ClientInfo `json:"clients"`
}

type StatsCount struct {
	Running int `json:"running"`
	Idle    int `json:"idle"`
	Error   int `json:"error"`
}

type ServerConfig struct {
	Name    string     `json:"name"`
	Mode    ServerMode `json:"mode"`
	Command string     `json:"command,omitempty"`
	Args    []string   `json:"args,omitempty"`
	URL     string     `json:"url,omitempty"`
	Env     []string   `json:"env,omitempty"`
	Lazy    bool       `json:"lazy"`
}

type SyncResult struct {
	ClientID string `json:"clientID"`
	OK       bool   `json:"ok"`
	Changes  int    `json:"changes"`
	Errors   string `json:"errors,omitempty"`
	DryRun   bool   `json:"dryRun"`
}

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Server  string    `json:"server"`
	Message string    `json:"message"`
}

type LogFilter struct {
	Level  string `json:"level"`
	Server string `json:"server"`
	Limit  int    `json:"limit"`
}

type ImportResult struct {
	Imported int      `json:"imported"`
	Errors   []string `json:"errors,omitempty"`
}
