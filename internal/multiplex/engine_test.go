package multiplex_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/multiplex"
	"github.com/harisbinrahat/mcp-hub/internal/protocol"
)

func TestForwardRequestRemapsCollidingIDs(t *testing.T) {
	engine := multiplex.NewEngine(nil, nil)

	var (
		mu          sync.Mutex
		internalIDs []string
	)

	send := func(raw []byte) error {
		env, err := protocol.Parse(raw)
		if err != nil {
			return err
		}
		if env.ID == nil {
			return nil
		}

		var payload struct {
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}

		mu.Lock()
		internalIDs = append(internalIDs, string(*env.ID))
		mu.Unlock()

		var decodedID any
		if err := json.Unmarshal(*env.ID, &decodedID); err != nil {
			return err
		}

		value := payload.Params["value"]
		go func() {
			response, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      decodedID,
				"result": map[string]any{
					"value": value,
				},
			})
			_, _ = engine.HandleBackendMessage(response)
		}()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type result struct {
		raw []byte
		err error
	}

	outA := make(chan result, 1)
	outB := make(chan result, 1)

	go func() {
		raw, err := engine.ForwardRequest(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"value":"a"}}`), send)
		outA <- result{raw: raw, err: err}
	}()
	go func() {
		raw, err := engine.ForwardRequest(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"value":"b"}}`), send)
		outB <- result{raw: raw, err: err}
	}()

	resA := <-outA
	resB := <-outB
	if resA.err != nil {
		t.Fatalf("request A failed: %v", resA.err)
	}
	if resB.err != nil {
		t.Fatalf("request B failed: %v", resB.err)
	}

	var parsedA, parsedB map[string]any
	if err := json.Unmarshal(resA.raw, &parsedA); err != nil {
		t.Fatalf("unmarshal response A: %v", err)
	}
	if err := json.Unmarshal(resB.raw, &parsedB); err != nil {
		t.Fatalf("unmarshal response B: %v", err)
	}

	if parsedA["id"].(float64) != 1 || parsedB["id"].(float64) != 1 {
		t.Fatalf("expected original ids to be preserved, got %v and %v", parsedA["id"], parsedB["id"])
	}

	gotValues := map[string]bool{
		parsedA["result"].(map[string]any)["value"].(string): true,
		parsedB["result"].(map[string]any)["value"].(string): true,
	}
	if !gotValues["a"] || !gotValues["b"] {
		t.Fatalf("expected both values to round-trip, got %#v", gotValues)
	}

	if len(internalIDs) != 2 || internalIDs[0] == internalIDs[1] || internalIDs[0] == "1" || internalIDs[1] == "1" {
		t.Fatalf("expected distinct internal ids, got %#v", internalIDs)
	}
}
