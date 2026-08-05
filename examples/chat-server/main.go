// Command chat-server runs the AI SDK v7 conformance chat endpoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"iter"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/aisdkhttp"
	"github.com/open-ai-sdk/ai-go/llm"
)

const (
	toolCallID = "tool-call-1"
	approvalID = "approval-1"
)

type textModel struct {
	fail bool
}

func (textModel) ModelID() string { return "conformance-text-model" }

func (model textModel) Stream(ctx context.Context, _ llm.Request) (<-chan aikit.StreamEvent, error) {
	events := make(chan aikit.StreamEvent, 2)
	go func() {
		defer close(events)
		stream := []aikit.StreamEvent{
			{Type: aikit.StreamEventTextDelta, TextDelta: "Hello from ai-go"},
			{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop},
		}
		if model.fail {
			stream[1] = aikit.StreamEvent{Type: aikit.StreamEventError, Error: errors.New("conformance stream error")}
		}
		for _, event := range stream {
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
	mux.Handle("/chat", scenarioHandler())
	mux.Handle("/ag-ui", aguiScenarioHandler())

	fmt.Printf("LISTEN http://%s\n", listener.Addr())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func scenarioHandler() http.Handler {
	handlers := map[string]http.Handler{
		"text":     aisdkhttp.Handler(agentRun(textModel{})),
		"error":    aisdkhttp.Handler(agentRun(textModel{fail: true})),
		"tool":     aisdkhttp.Handler(toolRun),
		"approval": aisdkhttp.Handler(approvalRun),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scenario := r.URL.Query().Get("scenario")
		if scenario == "" {
			scenario = "text"
		}
		handler, ok := handlers[scenario]
		if !ok {
			http.Error(w, "unknown scenario", http.StatusBadRequest)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func agentRun(model llm.Model) aisdkhttp.RunFunc {
	configured, err := agent.New(model).Build()
	if err != nil {
		panic(err)
	}
	return func(ctx context.Context, messages []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
		return configured.Runner().Messages(messages...).Stream(ctx)
	}
}

func toolRun(_ context.Context, messages []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
	if hasToolResult(messages, "tool-ok") {
		return eventStream(textEvents("Tool round-trip complete")...), nil
	}
	return eventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{
			Type: aikit.StepEventToolCallStart, ToolCallID: toolCallID,
			ToolCallName: "echo", ToolCallArgsDelta: `{"value":"ping"}`,
		},
		aikit.StepEvent{
			Type: aikit.StepEventToolCallReady, ToolCallID: toolCallID,
			ToolCallName: "echo", ToolCallArgsDelta: `{"value":"ping"}`,
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonToolCalls},
		aikit.StepEvent{Type: aikit.StepEventDone},
	), nil
}

func approvalRun(_ context.Context, messages []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
	if approved, ok := approvalResponse(messages); ok {
		text := "Approval denied"
		resultEvents := []aikit.StepEvent{
			{
				Type: aikit.StepEventToolCallReady, ToolCallID: toolCallID,
				ToolCallName: "dangerous_action", ToolCallArgsDelta: `{"target":"fixture"}`,
			},
			{Type: aikit.StepEventToolOutputDenied, ToolCallID: toolCallID},
		}
		if approved {
			text = "Approval accepted"
			// The approval decision itself is the result exercised by this
			// fixture. Starting another completed tool part in the fresh response
			// would make useChat's tool-call auto-submit predicate fire again.
			resultEvents = nil
		}
		events := append([]aikit.StepEvent{{Type: aikit.StepEventStepStart}}, resultEvents...)
		events = append(events, textEvents(text)[1:]...)
		return eventStream(events...), nil
	}

	return eventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{
			Type: aikit.StepEventToolCallStart, ToolCallID: toolCallID,
			ToolCallName: "dangerous_action", ToolCallArgsDelta: `{"target":"fixture"}`,
		},
		aikit.StepEvent{
			Type: aikit.StepEventToolCallReady, ToolCallID: toolCallID,
			ToolCallName: "dangerous_action", ToolCallArgsDelta: `{"target":"fixture"}`,
		},
		aikit.StepEvent{
			Type: aikit.StepEventToolApprovalRequest, ApprovalID: approvalID,
			ToolCallID: toolCallID, ToolCallName: "dangerous_action",
			ToolCallArgsDelta: `{"target":"fixture"}`,
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonToolCalls},
		aikit.StepEvent{Type: aikit.StepEventDone},
	), nil
}

func textEvents(text string) []aikit.StepEvent {
	return []aikit.StepEvent{
		{Type: aikit.StepEventStepStart},
		{Type: aikit.StepEventTextDelta, TextDelta: text},
		{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		{Type: aikit.StepEventDone},
	}
}

func eventStream(events ...aikit.StepEvent) iter.Seq2[aikit.StepEvent, error] {
	return func(yield func(aikit.StepEvent, error) bool) {
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func hasToolResult(messages []aikit.Message, expected string) bool {
	for _, message := range messages {
		for _, part := range message.Content {
			if part.Type == aikit.ContentPartTypeToolResult && strings.Contains(part.ToolResultOutput, expected) {
				return true
			}
		}
	}
	return false
}

func approvalResponse(messages []aikit.Message) (bool, bool) {
	for _, message := range messages {
		for _, part := range message.Content {
			if part.Type == aikit.ContentPartTypeToolApprovalResponse && part.ToolApprovalID == approvalID {
				return part.ToolApprovalApproved, true
			}
		}
	}
	return false, false
}
