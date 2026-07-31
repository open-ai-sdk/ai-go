package mcp

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestHTTPTransport_ReadSSEStream_LargeToolResult(t *testing.T) {
	largeText := strings.Repeat("large tool result ", 16*1024)
	result := map[string]any{
		"content": []map[string]any{{
			"type": "text",
			"text": largeText,
		}},
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	message := NewResponse(IntID(1), resultJSON)
	payload, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}

	var received []JSONRPCMessage
	var handlerErrors []error
	transport := NewHTTPTransport(HTTPTransportConfig{
		URL: "https://example.test/mcp",
	})
	transport.SetHandlers(
		func(message JSONRPCMessage) {
			received = append(received, message)
		},
		nil,
		func(err error) {
			handlerErrors = append(handlerErrors, err)
		},
	)

	transport.readSSEStream(io.NopCloser(strings.NewReader(
		"event: message\ndata: " + string(payload) + "\n\n",
	)))

	if len(handlerErrors) != 0 {
		t.Fatalf("handler errors = %v", handlerErrors)
	}
	if len(received) != 1 {
		t.Fatalf("message count = %d, want 1", len(received))
	}
	if received[0].Response == nil || received[0].Response.ID.String() != "1" {
		t.Fatalf("response ID = %#v, want 1", received[0].Response)
	}

	var decoded struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(received[0].Response.Result, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Content) != 1 || decoded.Content[0].Text != largeText {
		t.Fatalf("large tool result was truncated or changed")
	}
}
