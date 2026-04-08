package hub

import (
	"testing"

	"github.com/harisbinrahat/mcp-hub/internal/config"
)

func TestResolveReverseTargetSessionUsesProgressToken(t *testing.T) {
	s := newSharedServer("fixture", testSharedDef(), nil)
	s.sessions["s1"] = &session{initialized: true, subscribers: map[uint64]chan []byte{}}
	s.sessions["s2"] = &session{initialized: true, subscribers: map[uint64]chan []byte{}}
	s.activeReverseSession = "s2"
	s.progressByToken["tok-1"] = "s1"

	raw := []byte(`{"jsonrpc":"2.0","id":"r1","method":"sampling/createMessage","params":{"_meta":{"progressToken":"tok-1"}}}`)
	got, err := s.resolveReverseTargetSession(raw)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if got != "s1" {
		t.Fatalf("expected token-mapped session s1, got %q", got)
	}
}

func TestResolveReverseTargetSessionUsesActiveSession(t *testing.T) {
	s := newSharedServer("fixture", testSharedDef(), nil)
	s.sessions["s1"] = &session{initialized: true, subscribers: map[uint64]chan []byte{}}
	s.activeReverseSession = "s1"

	raw := []byte(`{"jsonrpc":"2.0","id":"r1","method":"roots/list","params":{}}`)
	got, err := s.resolveReverseTargetSession(raw)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if got != "s1" {
		t.Fatalf("expected active session s1, got %q", got)
	}
}

func TestResolveReverseTargetSessionFallsBackToSingleInitialized(t *testing.T) {
	s := newSharedServer("fixture", testSharedDef(), nil)
	s.sessions["s1"] = &session{initialized: true, subscribers: map[uint64]chan []byte{}}
	s.sessions["s2"] = &session{initialized: false, subscribers: map[uint64]chan []byte{}}

	raw := []byte(`{"jsonrpc":"2.0","id":"r1","method":"elicitation/create","params":{}}`)
	got, err := s.resolveReverseTargetSession(raw)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if got != "s1" {
		t.Fatalf("expected single initialized session s1, got %q", got)
	}
}

func TestResolveReverseTargetSessionErrorsOnAmbiguousSessions(t *testing.T) {
	s := newSharedServer("fixture", testSharedDef(), nil)
	s.sessions["s1"] = &session{initialized: true, subscribers: map[uint64]chan []byte{}}
	s.sessions["s2"] = &session{initialized: true, subscribers: map[uint64]chan []byte{}}

	raw := []byte(`{"jsonrpc":"2.0","id":"r1","method":"roots/list","params":{}}`)
	_, err := s.resolveReverseTargetSession(raw)
	if err == nil {
		t.Fatalf("expected ambiguity error")
	}
}

func testSharedDef() config.ServerConfig {
	return config.ServerConfig{
		Mode: config.ModeShared,
	}
}
