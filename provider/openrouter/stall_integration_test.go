package openrouter_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/provider/openrouter"
)

// A server that emits only keepalive comments defeats the byte-level
// ChunkTimeout (every comment is a byte) but must trip WithStallTimeout.
func TestStallTimeoutNotDefeatedByKeepaliveComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Millisecond):
				fmt.Fprint(w, ": OPENROUTER PROCESSING\n\n")
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	model := openrouter.NewLanguageModel("example/model", openrouter.Config{
		BaseURL:      server.URL,
		ChunkTimeout: 2 * time.Second, // bytes arrive every 5ms, so this never fires
	})
	stalled := llm.WithStallTimeout(model, 100*time.Millisecond)

	start := time.Now()
	events, err := stalled.Stream(context.Background(), llm.Request{Messages: []aikit.Message{aikit.UserMessage("hi")}})
	if err != nil {
		t.Fatal(err)
	}
	var streamErr error
	for ev := range events {
		if ev.Type == aikit.StreamEventError {
			streamErr = ev.Error
		}
	}
	var stall *llm.StallTimeoutError
	if !errors.As(streamErr, &stall) {
		t.Fatalf("err = %v, want StallTimeoutError", streamErr)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("stall detection took %v", elapsed)
	}
}
