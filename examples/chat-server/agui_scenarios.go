package main

import (
	"context"
	"iter"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/aisdkhttp"
	"github.com/open-ai-sdk/ai-go/uistream/agui"
)

// aguiScenarioHandler serves the same fixtures as /chat over AG-UI, so one set
// of engine events is proven against both wire protocols.
func aguiScenarioHandler() http.Handler {
	protocol := agui.Protocol(agui.WithRunID(func() string { return "run-conformance" }))
	handlers := map[string]http.Handler{
		"text":      aisdkhttp.HandlerFor(protocol, agentRun(textModel{})),
		"error":     aisdkhttp.HandlerFor(protocol, agentRun(textModel{fail: true})),
		"tool":      aisdkhttp.HandlerFor(protocol, toolRun),
		"approval":  aisdkhttp.HandlerFor(protocol, approvalRun),
		"reasoning": aisdkhttp.HandlerFor(protocol, reasoningRun),
		"rich":      aisdkhttp.HandlerFor(protocol, richRun),
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

// reasoningRun exercises the REASONING_* block, which must open and close
// before the assistant text begins.
func reasoningRun(_ context.Context, _ []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
	return eventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventReasoningDelta, ReasoningDelta: "Weighing the options"},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "Reasoned answer"},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	), nil
}

// richRun covers the events that reach AG-UI as CUSTOM, plus usage folding.
func richRun(_ context.Context, _ []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
	return eventStream(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "Answer with citations"},
		aikit.StepEvent{
			Type: aikit.StepEventSource,
			Source: &aikit.Source{
				SourceType: "url", ID: "src-1",
				URL: "https://example.test/a", Title: "Example",
			},
		},
		aikit.StepEvent{
			Type: aikit.StepEventFileDelta, FileData: []byte("PNGDATA"), FileMediaType: "image/png",
		},
		aikit.StepEvent{Type: aikit.StepEventStructuredOutput, StructuredOutput: []byte(`{"ok":true}`)},
		aikit.StepEvent{
			Type:  aikit.StepEventUsage,
			Usage: &aikit.Usage{InputTokens: 11, OutputTokens: 22, TotalTokens: 33},
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	), nil
}
