package multiplex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/harisbinrahat/mcp-hub/internal/protocol"
)

type NotificationHandler func(raw []byte)
type ReverseRequestHandler func(raw []byte) ([]byte, error)

type pendingRequest struct {
	originalID json.RawMessage
	responseCh chan []byte
}

type Engine struct {
	mu               sync.Mutex
	counter          uint64
	pending          map[string]pendingRequest
	onNotification   NotificationHandler
	onReverseRequest ReverseRequestHandler
}

func NewEngine(onNotification NotificationHandler, onReverseRequest ReverseRequestHandler) *Engine {
	return &Engine{
		pending:          map[string]pendingRequest{},
		onNotification:   onNotification,
		onReverseRequest: onReverseRequest,
	}
}

func (e *Engine) ForwardRequest(ctx context.Context, raw []byte, send func([]byte) error) ([]byte, error) {
	env, err := protocol.Parse(raw)
	if err != nil {
		return nil, err
	}

	if !protocol.IsRequest(env) {
		if err := send(raw); err != nil {
			return nil, err
		}
		return nil, nil
	}

	internalID, responseCh := e.register(*env.ID)
	forwarded, err := protocol.ReplaceID(raw, internalID)
	if err != nil {
		e.unregister(string(internalID))
		return nil, err
	}
	if err := send(forwarded); err != nil {
		e.unregister(string(internalID))
		return nil, err
	}

	select {
	case response := <-responseCh:
		return response, nil
	case <-ctx.Done():
		e.unregister(string(internalID))
		return nil, ctx.Err()
	}
}

func (e *Engine) HandleBackendMessage(raw []byte) ([]byte, error) {
	env, err := protocol.Parse(raw)
	if err != nil {
		return nil, err
	}

	switch {
	case protocol.IsNotification(env):
		if e.onNotification != nil {
			e.onNotification(raw)
		}
		return nil, nil
	case protocol.IsRequest(env):
		if e.onReverseRequest != nil {
			return e.onReverseRequest(raw)
		}
		return protocol.NewErrorResponse(*env.ID, -32601, "server-to-client requests are not supported yet")
	case protocol.IsResponse(env):
		key := string(*env.ID)
		e.mu.Lock()
		pending, ok := e.pending[key]
		if ok {
			delete(e.pending, key)
		}
		e.mu.Unlock()
		if !ok {
			return nil, nil
		}
		rewritten, err := protocol.ReplaceID(raw, pending.originalID)
		if err != nil {
			return nil, err
		}
		select {
		case pending.responseCh <- rewritten:
		default:
			return nil, errors.New("response channel blocked")
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown backend message shape")
	}
}

func (e *Engine) register(originalID json.RawMessage) (json.RawMessage, chan []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.counter++
	internalID := protocol.InternalID(e.counter)
	responseCh := make(chan []byte, 1)
	e.pending[string(internalID)] = pendingRequest{
		originalID: protocol.CloneRaw(originalID),
		responseCh: responseCh,
	}
	return internalID, responseCh
}

func (e *Engine) unregister(internalKey string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.pending, internalKey)
}
