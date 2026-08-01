package aisdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

type invariantLogRecorder struct {
	mu    sync.Mutex
	count int
}

func (*invariantLogRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *invariantLogRecorder) Handle(context.Context, slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
	return nil
}
func (r *invariantLogRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *invariantLogRecorder) WithGroup(string) slog.Handler      { return r }

func TestInvariantWithoutLoggerDoesNotUseSlogDefault(t *testing.T) {
	recorder := &invariantLogRecorder{}
	previous := slog.Default()
	slog.SetDefault(slog.New(recorder))
	t.Cleanup(func() { slog.SetDefault(previous) })

	reportInvariant(nil, nil, InvariantViolation{Code: InvariantUnknownChunk})
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.count != 0 {
		t.Fatalf("slog.Default received %d records without an injected logger", recorder.count)
	}
}

func TestInvariantCheckerRejectsLenientClientCases(t *testing.T) {
	tests := []struct {
		name   string
		chunks []Chunk
		code   InvariantCode
	}{
		{
			"text delta without start",
			[]Chunk{{Type: ChunkTextDelta, Fields: map[string]any{"id": "x"}}},
			InvariantBlockWithoutStart,
		},
		{
			"reasoning end without start",
			[]Chunk{{Type: ChunkReasoningEnd, Fields: map[string]any{"id": "x"}}},
			InvariantBlockWithoutStart,
		},
		{"open block", []Chunk{{Type: ChunkTextStart, Fields: map[string]any{"id": "x"}}}, InvariantBlockStillOpen},
		{"duplicate tool id", []Chunk{
			{Type: ChunkToolInputStart, Fields: toolFields("call-1", "one")},
			{Type: ChunkToolInputStart, Fields: toolFields("call-1", "two")},
		}, InvariantDuplicateToolCall},
		{
			"empty tool id",
			[]Chunk{{Type: ChunkToolInputStart, Fields: toolFields("", "tool")}},
			InvariantEmptyToolCallID,
		},
		{
			"empty tool name",
			[]Chunk{
				{
					Type:   ChunkToolInputAvailable,
					Fields: map[string]any{"toolCallId": "call-1", "toolName": "", "input": nil},
				},
			},
			InvariantEmptyToolName,
		},
		{
			"missing input",
			[]Chunk{{Type: ChunkToolInputAvailable, Fields: toolFields("call-1", "tool")}},
			InvariantMissingToolInput,
		},
		{"unknown type", []Chunk{{Type: "future-mistake"}}, InvariantUnknownChunk},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := NewInvariantChecker()
			var codes []InvariantCode
			for _, chunk := range test.chunks {
				for _, violation := range checker.Observe(chunk) {
					codes = append(codes, violation.Code)
				}
			}
			for _, violation := range checker.Finalize() {
				codes = append(codes, violation.Code)
			}
			if !slices.Contains(codes, test.code) {
				t.Fatalf("codes = %v, want %s", codes, test.code)
			}
		})
	}
}

func TestInvariantCheckerAcceptsValidLifecycleAndDataPrefix(t *testing.T) {
	checker := NewInvariantChecker()
	chunks := []Chunk{
		{Type: ChunkStart},
		{Type: ChunkStartStep},
		{Type: ChunkTextStart, Fields: map[string]any{"id": "text-1"}},
		{Type: ChunkTextDelta, Fields: map[string]any{"id": "text-1", "delta": "hi"}},
		{Type: ChunkTextEnd, Fields: map[string]any{"id": "text-1"}},
		{Type: ChunkToolInputStart, Fields: toolFields("call-1", "lookup")},
		{Type: ChunkToolInputDelta, Fields: map[string]any{"toolCallId": "call-1", "inputTextDelta": "{}"}},
		{
			Type:   ChunkToolInputAvailable,
			Fields: map[string]any{"toolCallId": "call-1", "toolName": "lookup", "input": nil},
		},
		{Type: ChunkToolOutputAvailable, Fields: map[string]any{"toolCallId": "call-1", "output": "ok"}},
		{Type: "data-plan", Fields: map[string]any{"data": nil}},
		{Type: ChunkFinishStep},
		{Type: ChunkFinish},
	}
	for _, chunk := range chunks {
		if violations := checker.Observe(chunk); len(violations) != 0 {
			t.Fatalf("chunk %#v: %v", chunk, violations)
		}
	}
	if violations := checker.Finalize(); len(violations) != 0 {
		t.Fatal(violations)
	}
	if !ValidChunkType("data-") || ValidChunkType("source") {
		t.Fatal("data-* or literal union guard is incorrect")
	}
}

func TestProducerReportsViolationWithoutDroppingStream(t *testing.T) {
	events := make(chan aikit.StepEvent, 4)
	events <- aikit.StepEvent{Type: aikit.StepEventStepStart}
	events <- aikit.StepEvent{Type: aikit.StepEventToolCallStart}
	events <- aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop}
	events <- aikit.StepEvent{Type: aikit.StepEventDone}
	close(events)
	var violations []InvariantViolation
	producer := NewChunkProducer("message-1", WithInvariantReporter(func(v InvariantViolation) {
		violations = append(violations, v)
	}))
	chunks := drainChunks(producer.Produce(events).Chunks)
	if len(violations) == 0 || violations[0].Code != InvariantEmptyToolCallID {
		t.Fatalf("violations = %#v", violations)
	}
	if _, ok := findChunk(chunks, ChunkFinish); !ok {
		t.Fatalf("violating event dropped the remaining stream: %#v", chunks)
	}
}

func TestReporterPanicDoesNotBreakProducer(t *testing.T) {
	events := make(chan aikit.StepEvent, 2)
	events <- aikit.StepEvent{Type: aikit.StepEventToolCallStart}
	events <- aikit.StepEvent{Type: aikit.StepEventDone}
	close(events)
	chunks := drainChunks(NewChunkProducer("message-1", WithInvariantReporter(func(InvariantViolation) {
		panic("observer bug")
	})).Produce(events).Chunks)
	if _, ok := findChunk(chunks, ChunkFinish); !ok {
		t.Fatalf("reporter panic terminated producer: %#v", chunks)
	}
}

func TestCommittedFixturesPassInvariants(t *testing.T) {
	fixtures, err := filepath.Glob("testdata/*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range fixtures {
		t.Run(filepath.Base(path), func(t *testing.T) {
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			checker := NewInvariantChecker()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				payload := strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "data: ")
				if payload == "" || payload == "[DONE]" {
					continue
				}
				var fields map[string]any
				if err := json.Unmarshal([]byte(payload), &fields); err != nil {
					t.Fatal(err)
				}
				typ, _ := fields["type"].(string)
				delete(fields, "type")
				if violations := checker.Observe(Chunk{Type: typ, Fields: fields}); len(violations) != 0 {
					t.Fatalf("%s: %v", payload, violations)
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			if violations := checker.Finalize(); len(violations) != 0 {
				t.Fatal(violations)
			}
		})
	}
}

func TestRandomValidStepEventsSatisfyInvariants(t *testing.T) {
	var nodeChunks []map[string]any
	for seed := uint64(1); seed <= 100; seed++ {
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		events := make(chan aikit.StepEvent, 128)
		enqueueValidRandomEvents(events, rng, seed)
		close(events)
		var violations []InvariantViolation
		stream := NewChunkProducer("property", WithInvariantReporter(func(v InvariantViolation) {
			violations = append(violations, v)
		})).Produce(events)
		for chunk := range stream.Chunks {
			payload := make(map[string]any, len(chunk.Fields)+1)
			for key, value := range chunk.Fields {
				payload[key] = value
			}
			payload["type"] = chunk.Type
			nodeChunks = append(nodeChunks, payload)
		}
		if len(violations) != 0 {
			t.Fatalf("seed %d: %v", seed, violations)
		}
	}
	payload, err := json.Marshal(nodeChunks)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bun", "../conformance/src/validate_chunks.ts")
	command.Stdin = bytes.NewReader(payload)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Node schema rejected generated producer chunks: %v\n%s", err, output)
	}
}

func enqueueValidRandomEvents(events chan<- aikit.StepEvent, rng *rand.Rand, seed uint64) {
	toolNumber := 0
	steps := 1 + rng.IntN(3)
	for step := 0; step < steps; step++ {
		events <- aikit.StepEvent{Type: aikit.StepEventStepStart, StepNumber: step}
		for block := 0; block < 2+rng.IntN(7); block++ {
			switch rng.IntN(4) {
			case 0:
				events <- aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "text"}
			case 1:
				events <- aikit.StepEvent{Type: aikit.StepEventReasoningDelta, ReasoningDelta: "thought", ThoughtSignature: "sig"}
			case 2:
				toolNumber++
				id := fmt.Sprintf("call-%d-%d", seed, toolNumber)
				events <- aikit.StepEvent{Type: aikit.StepEventToolCallStart, ToolCallID: id, ToolCallName: "lookup", ToolCallArgsDelta: `{"value":`}
				events <- aikit.StepEvent{Type: aikit.StepEventToolCallDelta, ToolCallID: id, ToolCallName: "lookup", ToolCallArgsDelta: fmt.Sprintf("%d}", toolNumber)}
				events <- aikit.StepEvent{Type: aikit.StepEventToolCallReady, ToolCallID: id, ToolCallName: "lookup"}
				events <- aikit.StepEvent{Type: aikit.StepEventToolResult, ToolResult: &aikit.ToolResult{ID: id, Name: "lookup", Args: fmt.Sprintf(`{"value":%d}`, toolNumber), Output: `{"ok":true}`}}
			case 3:
				events <- aikit.StepEvent{Type: aikit.StepEventSource, Source: &aikit.Source{ID: fmt.Sprintf("source-%d-%d", seed, block), URL: "https://example.com", Title: "Example"}}
			}
		}
		if step == steps-1 && rng.IntN(8) == 0 {
			events <- aikit.StepEvent{Type: aikit.StepEventError, Error: errors.New("generated failure")}
			return
		}
		events <- aikit.StepEvent{Type: aikit.StepEventStepEnd, StepNumber: step, FinishReason: aikit.FinishReasonStop}
	}
	events <- aikit.StepEvent{Type: aikit.StepEventDone}
}

func toolFields(id, name string) map[string]any {
	return map[string]any{"toolCallId": id, "toolName": name}
}
