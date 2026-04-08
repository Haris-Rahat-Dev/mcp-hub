package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type envelope struct {
	JSONRPC string           `json:"jsonrpc,omitempty"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
}

var (
	reverseMu      sync.Mutex
	reverseWaiters = map[string]json.RawMessage{}
	reverseCounter uint64
)

func main() {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)

	for {
		var msg envelope
		if err := decoder.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			fmt.Fprintf(os.Stderr, "decode error: %v\n", err)
			os.Exit(1)
		}

		switch msg.Method {
		case "":
			if msg.ID != nil && deliverReverseResponse(encoder, *msg.ID, msg) {
				continue
			}
			respond(encoder, msg.ID, nil, map[string]any{
				"code":    -32601,
				"message": "unknown response id",
			})
		case "initialize":
			respond(encoder, msg.ID, map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "fixture",
					"version": "0.1.0",
				},
			}, nil)
		case "notifications/initialized", "initialized":
		case "ping":
			respond(encoder, msg.ID, map[string]any{}, nil)
		case "tools/list":
			respond(encoder, msg.ID, map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echoes arguments back",
						"inputSchema": map[string]any{"type": "object"},
					},
					{
						"name":        "notify",
						"description": "Emits a notification",
						"inputSchema": map[string]any{"type": "object"},
					},
					{
						"name":        "delay",
						"description": "Sleeps before responding",
						"inputSchema": map[string]any{"type": "object"},
					},
					{
						"name":        "crash",
						"description": "Exits after responding",
						"inputSchema": map[string]any{"type": "object"},
					},
					{
						"name":        "reverse",
						"description": "Sends a reverse roots/list request",
						"inputSchema": map[string]any{"type": "object"},
					},
				},
			}, nil)
		case "tools/call":
			handleToolCall(encoder, msg)
		default:
			respond(encoder, msg.ID, nil, map[string]any{
				"code":    -32601,
				"message": "method not found",
			})
		}
	}
}

func handleToolCall(encoder *json.Encoder, msg envelope) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		respond(encoder, msg.ID, nil, map[string]any{
			"code":    -32602,
			"message": err.Error(),
		})
		return
	}

	switch params.Name {
	case "echo":
		respond(encoder, msg.ID, map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": fmt.Sprintf("%v", params.Arguments["value"]),
				},
			},
			"echo": params.Arguments,
		}, nil)
	case "notify":
		_ = encoder.Encode(map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/message",
			"params": map[string]any{
				"level": "info",
				"data":  params.Arguments["value"],
			},
		})
		respond(encoder, msg.ID, map[string]any{
			"ok": true,
		}, nil)
	case "delay":
		if ms, ok := params.Arguments["ms"].(float64); ok {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		respond(encoder, msg.ID, map[string]any{
			"ok": true,
		}, nil)
	case "crash":
		respond(encoder, msg.ID, map[string]any{
			"ok": true,
		}, nil)
		go func() {
			time.Sleep(50 * time.Millisecond)
			os.Exit(91)
		}()
	case "reverse":
		reverseID := nextReverseID()
		registerReverseWaiter(reverseID, *msg.ID)
		_ = encoder.Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      reverseID,
			"method":  "roots/list",
			"params":  map[string]any{},
		})
		// Response for this tools/call is sent later when the reverse response arrives.
		return
	default:
		respond(encoder, msg.ID, nil, map[string]any{
			"code":    -32602,
			"message": "unknown tool",
		})
	}
}

func nextReverseID() string {
	n := atomic.AddUint64(&reverseCounter, 1)
	return "reverse_" + strconv.FormatUint(n, 10)
}

func registerReverseWaiter(id string, requestID json.RawMessage) {
	reverseMu.Lock()
	defer reverseMu.Unlock()
	cloned := make(json.RawMessage, len(requestID))
	copy(cloned, requestID)
	reverseWaiters[`"`+id+`"`] = cloned
}

func deliverReverseResponse(encoder *json.Encoder, rawID json.RawMessage, msg envelope) bool {
	reverseMu.Lock()
	requestID, ok := reverseWaiters[string(rawID)]
	if !ok {
		reverseMu.Unlock()
		return false
	}
	delete(reverseWaiters, string(rawID))
	reverseMu.Unlock()

	var result any
	if len(msg.Result) > 0 {
		_ = json.Unmarshal(msg.Result, &result)
	}
	respond(encoder, &requestID, map[string]any{
		"ok":               true,
		"reverse_response": result,
	}, nil)
	return true
}

func respond(encoder *json.Encoder, id *json.RawMessage, result any, rpcError any) {
	payload := map[string]any{
		"jsonrpc": "2.0",
	}
	if id != nil {
		var decoded any
		if err := json.Unmarshal(*id, &decoded); err == nil {
			payload["id"] = decoded
		}
	}
	if rpcError != nil {
		payload["error"] = rpcError
	} else {
		payload["result"] = result
	}
	_ = encoder.Encode(payload)
}
