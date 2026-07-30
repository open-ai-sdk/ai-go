package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/open-ai-sdk/ai-go/ai"
	"github.com/open-ai-sdk/ai-go/uistream"
)

const (
	toolCallID = "tool-call-1"
	approvalID = "approval-1"
)

type textModel struct {
	fail bool
}

func (textModel) ModelID() string { return "conformance-text-model" }

func (m textModel) Stream(ctx context.Context, _ ai.LanguageModelRequest) (<-chan ai.StreamEvent, error) {
	streamEvents := []ai.StreamEvent{
		{Type: ai.StreamEventTextDelta, TextDelta: "Hello from ai-go"},
		{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop},
	}
	if m.fail {
		streamEvents[1] = ai.StreamEvent{
			Type:  ai.StreamEventError,
			Error: errors.New("conformance stream error"),
		}
	}
	events := make(chan ai.StreamEvent, len(streamEvents))
	go func() {
		defer close(events)
		for _, event := range streamEvents {
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address")
	flag.Parse()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("scenario") {
		case "", "text":
			serveText(w, r, false)
		case "error":
			serveText(w, r, true)
		case "tool":
			serveTool(w, r)
		case "approval":
			serveApproval(w, r)
		default:
			http.Error(w, "unknown scenario", http.StatusBadRequest)
		}
	})

	fmt.Printf("LISTEN http://%s\n", listener.Addr())
	if err := http.Serve(listener, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func serveText(w http.ResponseWriter, r *http.Request, fail bool) {
	var envelope uistream.ChatRequestEnvelope
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	result := ai.StreamText(r.Context(), ai.GenerateTextRequest{
		Model:    textModel{fail: fail},
		Messages: uistream.ToAIMessages(envelope.Messages),
	})
	uistream.SetUIMessageStreamHeaders(w.Header())
	chunks := uistream.ToUIMessageStream(
		result,
		uistream.ResolveMessageIDFromEnvelope(envelope, "assistant-text"),
		uistream.ToUIStreamOptions{},
	)
	if err := uistream.WriteSSEStream(w, chunks); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func serveTool(w http.ResponseWriter, r *http.Request) {
	body := decodeBody(r)
	uistream.SetUIMessageStreamHeaders(w.Header())
	writer := uistream.NewWriter(w)

	if containsKeyValue(body, "result", "tool-ok") {
		writeText(writer, "Tool round-trip complete")
		return
	}

	mustWrite(writer.WriteStart("assistant-tool"))
	mustWrite(writer.WriteChunk(uistream.ChunkStartStep, nil))
	mustWrite(writer.WriteChunk(uistream.ChunkToolInputAvailable, map[string]any{
		"toolCallId": toolCallID,
		"toolName":   "echo",
		"input":      map[string]any{"value": "ping"},
	}))
	mustWrite(writer.WriteChunk(uistream.ChunkFinishStep, nil))
	mustWrite(writer.WriteFinishWithReason("tool-calls", nil))
}

func serveApproval(w http.ResponseWriter, r *http.Request) {
	body := decodeBody(r)
	uistream.SetUIMessageStreamHeaders(w.Header())
	writer := uistream.NewWriter(w)

	if approved, ok := findApprovalResponse(body); ok {
		mustWrite(writer.WriteStart("assistant-final"))
		mustWrite(writer.WriteChunk(uistream.ChunkStartStep, nil))
		if approved {
			mustWrite(writer.WriteChunk(uistream.ChunkToolOutputAvailable, map[string]any{
				"toolCallId": toolCallID,
				"output":     map[string]any{"ok": true},
			}))
			writeTextBlock(writer, "Approval accepted")
		} else {
			mustWrite(writer.WriteToolOutputDenied(toolCallID, nil))
			writeTextBlock(writer, "Approval denied")
		}
		mustWrite(writer.WriteChunk(uistream.ChunkFinishStep, nil))
		mustWrite(writer.WriteFinishWithReason("stop", nil))
		return
	}

	mustWrite(writer.WriteStart("assistant-approval"))
	mustWrite(writer.WriteChunk(uistream.ChunkStartStep, nil))
	mustWrite(writer.WriteChunk(uistream.ChunkToolInputAvailable, map[string]any{
		"toolCallId": toolCallID,
		"toolName":   "dangerous_action",
		"input":      map[string]any{"target": "fixture"},
	}))
	mustWrite(writer.WriteToolApprovalRequest(
		approvalID,
		toolCallID,
		"dangerous_action",
		map[string]any{"target": "fixture"},
	))
	mustWrite(writer.WriteChunk(uistream.ChunkFinishStep, nil))
	mustWrite(writer.WriteFinishWithReason("tool-calls", nil))
}

func writeText(writer *uistream.Writer, text string) {
	mustWrite(writer.WriteStart("assistant-final"))
	mustWrite(writer.WriteChunk(uistream.ChunkStartStep, nil))
	writeTextBlock(writer, text)
	mustWrite(writer.WriteChunk(uistream.ChunkFinishStep, nil))
	mustWrite(writer.WriteFinishWithReason("stop", nil))
}

func writeTextBlock(writer *uistream.Writer, text string) {
	mustWrite(writer.WriteChunk(uistream.ChunkTextStart, map[string]any{"id": "text-1"}))
	mustWrite(writer.WriteChunk(uistream.ChunkTextDelta, map[string]any{
		"id":    "text-1",
		"delta": text,
	}))
	mustWrite(writer.WriteChunk(uistream.ChunkTextEnd, map[string]any{"id": "text-1"}))
}

func decodeBody(r *http.Request) any {
	defer r.Body.Close()
	var body any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil
	}
	return body
}

func containsKeyValue(value any, key, expected string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for currentKey, child := range typed {
			if currentKey == key && child == expected {
				return true
			}
			if containsKeyValue(child, key, expected) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsKeyValue(child, key, expected) {
				return true
			}
		}
	}
	return false
}

func findApprovalResponse(value any) (bool, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if typed["state"] == "approval-responded" {
			if approval, ok := typed["approval"].(map[string]any); ok &&
				approval["id"] == approvalID {
				approved, ok := approval["approved"].(bool)
				return approved, ok
			}
		}
		for _, child := range typed {
			if result, ok := findApprovalResponse(child); ok {
				return result, true
			}
		}
	case []any:
		for _, child := range typed {
			if result, ok := findApprovalResponse(child); ok {
				return result, true
			}
		}
	}
	return false, false
}

func mustWrite(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
