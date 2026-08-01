package uistream

import (
	"sync"
	"testing"
)

func TestHandleUIMessageStreamEnd_OnEndCalled(t *testing.T) {
	chunks := []Chunk{
		{Type: ChunkStart, Fields: map[string]any{"messageId": "msg-1"}},
		{Type: ChunkTextStart, Fields: map[string]any{"id": "t1"}},
		{Type: ChunkTextDelta, Fields: map[string]any{"id": "t1", "delta": "Hello World"}},
		{Type: ChunkTextEnd, Fields: map[string]any{"id": "t1"}},
		{Type: ChunkFinish, Fields: map[string]any{"finishReason": "stop"}},
	}

	var endInfo EndInfo
	var mu sync.Mutex

	out := HandleUIMessageStreamEnd(chunkSliceToChan(chunks), HandleUIMessageStreamEndOptions{
		MessageID: "msg-1",
		OnEnd: func(info EndInfo) {
			mu.Lock()
			endInfo = info
			mu.Unlock()
		},
	})
	drainChunks(out)

	mu.Lock()
	defer mu.Unlock()

	if endInfo.IsAborted {
		t.Error("expected IsAborted=false")
	}
	if endInfo.IsContinuation {
		t.Error("expected IsContinuation=false")
	}
	if endInfo.ResponseMessage.ID != "msg-1" {
		t.Errorf("expected message ID msg-1, got %q", endInfo.ResponseMessage.ID)
	}
	if endInfo.FinishReason != "stop" {
		t.Errorf("expected finishReason=stop, got %q", endInfo.FinishReason)
	}
	// Should have text part.
	if len(endInfo.ResponseMessage.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(endInfo.ResponseMessage.Parts))
	}
	if endInfo.ResponseMessage.Parts[0]["text"] != "Hello World" {
		t.Errorf("expected 'Hello World', got %v", endInfo.ResponseMessage.Parts[0]["text"])
	}
}

func TestHandleUIMessageStreamEnd_OnStepEndCalled(t *testing.T) {
	chunks := []Chunk{
		{Type: ChunkStart, Fields: map[string]any{"messageId": "msg-2"}},
		{Type: ChunkStartStep},
		{Type: ChunkTextStart, Fields: map[string]any{"id": "t1"}},
		{Type: ChunkTextDelta, Fields: map[string]any{"id": "t1", "delta": "Step 1"}},
		{Type: ChunkTextEnd, Fields: map[string]any{"id": "t1"}},
		{Type: ChunkFinishStep},
		{Type: ChunkStartStep},
		{Type: ChunkTextStart, Fields: map[string]any{"id": "t2"}},
		{Type: ChunkTextDelta, Fields: map[string]any{"id": "t2", "delta": "Step 2"}},
		{Type: ChunkTextEnd, Fields: map[string]any{"id": "t2"}},
		{Type: ChunkFinishStep},
		{Type: ChunkFinish, Fields: map[string]any{"finishReason": "stop"}},
	}

	var stepInfos []StepEndInfo
	var mu sync.Mutex

	out := HandleUIMessageStreamEnd(chunkSliceToChan(chunks), HandleUIMessageStreamEndOptions{
		MessageID: "msg-2",
		OnStepEnd: func(info StepEndInfo) {
			mu.Lock()
			stepInfos = append(stepInfos, info)
			mu.Unlock()
		},
	})
	drainChunks(out)

	mu.Lock()
	defer mu.Unlock()

	if len(stepInfos) != 2 {
		t.Fatalf("expected 2 step end callbacks, got %d", len(stepInfos))
	}

	// First step should have step-start + text part.
	step1Parts := stepInfos[0].ResponseMessage.Parts
	if len(step1Parts) != 2 { // step-start, text
		t.Fatalf("step 1: expected 2 parts, got %d: %v", len(step1Parts), step1Parts)
	}
	if step1Parts[0]["type"] != "step-start" {
		t.Errorf("step 1: expected step-start, got %v", step1Parts[0]["type"])
	}
	if step1Parts[1]["text"] != "Step 1" {
		t.Errorf("step 1: expected 'Step 1', got %v", step1Parts[1]["text"])
	}

	// Second step should have step-start*2 + text*2 (accumulated).
	step2Parts := stepInfos[1].ResponseMessage.Parts
	if len(step2Parts) != 4 { // step-start, text, step-start, text
		t.Fatalf("step 2: expected 4 parts, got %d: %v", len(step2Parts), step2Parts)
	}
}

func TestHandleUIMessageStreamEnd_AbortTracking(t *testing.T) {
	chunks := []Chunk{
		{Type: ChunkStart, Fields: map[string]any{"messageId": "msg-3"}},
		{Type: ChunkTextStart, Fields: map[string]any{"id": "t1"}},
		{Type: ChunkTextDelta, Fields: map[string]any{"id": "t1", "delta": "partial"}},
		{Type: ChunkAbort},
		{Type: ChunkFinish},
	}

	var endInfo EndInfo
	var mu sync.Mutex

	out := HandleUIMessageStreamEnd(chunkSliceToChan(chunks), HandleUIMessageStreamEndOptions{
		MessageID: "msg-3",
		OnEnd: func(info EndInfo) {
			mu.Lock()
			endInfo = info
			mu.Unlock()
		},
	})
	drainChunks(out)

	mu.Lock()
	defer mu.Unlock()

	if !endInfo.IsAborted {
		t.Error("expected IsAborted=true")
	}
}

func TestHandleUIMessageStreamEnd_Continuation(t *testing.T) {
	lastMsg := &StreamingUIMessage{
		ID:   "existing-assistant",
		Role: "assistant",
		Parts: []UIMessagePart{
			{"type": "text", "text": "previous "},
		},
	}

	chunks := []Chunk{
		{Type: ChunkStart}, // no messageId — should be injected
		{Type: ChunkStartStep},
		{Type: ChunkTextStart, Fields: map[string]any{"id": "t1"}},
		{Type: ChunkTextDelta, Fields: map[string]any{"id": "t1", "delta": "continued"}},
		{Type: ChunkTextEnd, Fields: map[string]any{"id": "t1"}},
		{Type: ChunkFinishStep},
		{Type: ChunkFinish},
	}

	var endInfo EndInfo
	var mu sync.Mutex

	out := HandleUIMessageStreamEnd(chunkSliceToChan(chunks), HandleUIMessageStreamEndOptions{
		MessageID:            "ignored-id",
		LastAssistantMessage: lastMsg,
		OnEnd: func(info EndInfo) {
			mu.Lock()
			endInfo = info
			mu.Unlock()
		},
	})

	// Verify messageId injection on start chunk.
	result := drainChunks(out)
	startChunk := result[0]
	if id, _ := startChunk.Fields["messageId"].(string); id != "existing-assistant" {
		t.Errorf("expected injected messageId=existing-assistant, got %q", id)
	}

	mu.Lock()
	defer mu.Unlock()

	if !endInfo.IsContinuation {
		t.Error("expected IsContinuation=true")
	}
	if endInfo.ResponseMessage.ID != "existing-assistant" {
		t.Errorf("expected message ID existing-assistant, got %q", endInfo.ResponseMessage.ID)
	}
}

func TestHandleUIMessageStreamEnd_MessageIdInjection(t *testing.T) {
	chunks := []Chunk{
		{Type: ChunkStart}, // no messageId
		{Type: ChunkFinish},
	}

	out := HandleUIMessageStreamEnd(chunkSliceToChan(chunks), HandleUIMessageStreamEndOptions{
		MessageID: "injected-123",
	})
	result := drainChunks(out)

	if id, _ := result[0].Fields["messageId"].(string); id != "injected-123" {
		t.Errorf("expected injected messageId=injected-123, got %q", id)
	}
}

func TestHandleUIMessageStreamEnd_ExistingMessageIdPreserved(t *testing.T) {
	chunks := []Chunk{
		{Type: ChunkStart, Fields: map[string]any{"messageId": "original-id"}},
		{Type: ChunkFinish},
	}

	out := HandleUIMessageStreamEnd(chunkSliceToChan(chunks), HandleUIMessageStreamEndOptions{
		MessageID: "should-not-override",
	})
	result := drainChunks(out)

	if id, _ := result[0].Fields["messageId"].(string); id != "original-id" {
		t.Errorf("expected preserved messageId=original-id, got %q", id)
	}
}

func TestHandleUIMessageStreamEnd_PassthroughWithoutCallbacks(t *testing.T) {
	chunks := []Chunk{
		{Type: ChunkStart, Fields: map[string]any{"messageId": "msg-pass"}},
		{Type: ChunkTextStart, Fields: map[string]any{"id": "t1"}},
		{Type: ChunkTextDelta, Fields: map[string]any{"id": "t1", "delta": "text"}},
		{Type: ChunkTextEnd, Fields: map[string]any{"id": "t1"}},
		{Type: ChunkFinish},
	}

	out := HandleUIMessageStreamEnd(chunkSliceToChan(chunks), HandleUIMessageStreamEndOptions{
		MessageID: "msg-pass",
		// No callbacks.
	})
	result := drainChunks(out)

	if len(result) != len(chunks) {
		t.Errorf("expected %d chunks passthrough, got %d", len(chunks), len(result))
	}
}

func TestHandleUIMessageStreamEnd_OnEndCalledOnce(t *testing.T) {
	chunks := []Chunk{
		{Type: ChunkStart, Fields: map[string]any{"messageId": "msg-once"}},
		{Type: ChunkFinish},
	}

	endCount := 0
	var mu sync.Mutex

	out := HandleUIMessageStreamEnd(chunkSliceToChan(chunks), HandleUIMessageStreamEndOptions{
		MessageID: "msg-once",
		OnEnd: func(info EndInfo) {
			mu.Lock()
			endCount++
			mu.Unlock()
		},
	})
	drainChunks(out)

	mu.Lock()
	defer mu.Unlock()

	if endCount != 1 {
		t.Errorf("expected onEnd called once, got %d", endCount)
	}
}
