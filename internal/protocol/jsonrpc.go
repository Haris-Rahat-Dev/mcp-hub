package protocol

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type Envelope struct {
	JSONRPC string           `json:"jsonrpc,omitempty"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError   `json:"error,omitempty"`
}

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func Parse(raw []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

func CloneRaw(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

func IsRequest(env Envelope) bool {
	return env.Method != "" && env.ID != nil
}

func IsNotification(env Envelope) bool {
	return env.Method != "" && env.ID == nil
}

func IsResponse(env Envelope) bool {
	return env.Method == "" && env.ID != nil
}

func ReplaceID(raw []byte, newID json.RawMessage) ([]byte, error) {
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	msg["id"] = CloneRaw(newID)
	return json.Marshal(msg)
}

func NewSuccessResponse(id json.RawMessage, result json.RawMessage) ([]byte, error) {
	var decodedID any
	if err := json.Unmarshal(id, &decodedID); err != nil {
		return nil, err
	}

	var decodedResult any
	if len(result) > 0 {
		if err := json.Unmarshal(result, &decodedResult); err != nil {
			return nil, err
		}
	}

	return json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      decodedID,
		"result":  decodedResult,
	})
}

func NewErrorResponse(id json.RawMessage, code int, message string) ([]byte, error) {
	var decodedID any
	if err := json.Unmarshal(id, &decodedID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      decodedID,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func InternalID(counter uint64) json.RawMessage {
	return json.RawMessage(strconv.AppendQuote(nil, fmt.Sprintf("hub_%d", counter)))
}
