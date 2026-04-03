package hub

import (
	"context"

	"github.com/harisbinrahat/mcp-hub/internal/config"
)

type plannedServer struct {
	name string
	def  config.ServerConfig
}

func newPlannedServer(name string, def config.ServerConfig) *plannedServer {
	return &plannedServer{name: name, def: def}
}

func (p *plannedServer) ensureSessionID(existing string) string { return existing }

func (p *plannedServer) handleRequest(_ context.Context, _ string, _ []byte) ([]byte, error) {
	return nil, ErrModeNotImplemented
}

func (p *plannedServer) subscribe(_ string) (<-chan []byte, func()) {
	ch := make(chan []byte)
	close(ch)
	return ch, func() {}
}

func (p *plannedServer) deleteSession(_ string) {}

func (p *plannedServer) statusSnapshot() ServerStatus {
	return ServerStatus{
		Name:      p.name,
		Mode:      p.def.Mode,
		Status:    "planned",
		PID:       0,
		Clients:   0,
		Restarts:  0,
		LastError: "",
		Lazy:      p.def.IsLazy(),
	}
}

func (p *plannedServer) sessionIDs() []string { return nil }

func (p *plannedServer) close() error { return nil }

var _ serverRuntime = (*plannedServer)(nil)

