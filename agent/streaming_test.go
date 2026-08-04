package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

// richRunScript exercises every Result field a single turn can populate, so
// "StreamRun equals Run" is a claim about the whole aggregate rather than about
// text alone.
func richRunScript() []aikit.StreamEvent {
	return []aikit.StreamEvent{
		{Type: aikit.StreamEventReasoningDelta, TextDelta: "weighing"},
		{Type: aikit.StreamEventTextDelta, TextDelta: "the "},
		{Type: aikit.StreamEventTextDelta, TextDelta: "answer"},
		{Type: aikit.StreamEventSource, Source: &aikit.Source{
			SourceType: "url", ID: "s1", URL: "https://example.test",
			ProviderMetadata: map[string]any{"rank": float64(1)},
		}},
		{Type: aikit.StreamEventFileDelta, FileData: []byte("png-bytes"), FileMediaType: "image/png"},
		{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{InputTokens: 12, TotalTokens: 12}},
		{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{OutputTokens: 5}},
		{
			Type:             aikit.StreamEventFinish,
			MessageID:        "msg-1",
			FinishReason:     aikit.FinishReasonStop,
			RawFinishReason:  "stop",
			ProviderMetadata: map[string]any{"openai": map[string]any{"responseId": "r1"}},
			Warnings:         []aikit.Warning{{Type: "other", Message: "careful"}},
		},
	}
}

// comparableStep drops aikit.ToolResult, whose typed Error field makes
// reflect.DeepEqual unreliable, and compares tool results by their identifying
// fields instead.
type comparableStep struct {
	MessageID        string
	Text             string
	Reasoning        string
	Content          []aikit.ContentPart
	ToolCalls        []aikit.ToolCallInfo
	ToolResults      []string
	Usage            aikit.Usage
	FinishReason     aikit.FinishReason
	RawFinishReason  string
	ProviderMetadata map[string]any
	Warnings         []aikit.Warning
	Sources          []aikit.Source
	Files            []agent.GeneratedFile
}

func comparableSteps(steps []agent.Step) []comparableStep {
	out := make([]comparableStep, len(steps))
	for i, step := range steps {
		out[i] = comparableStep{
			MessageID: step.MessageID, Text: step.Text, Reasoning: step.Reasoning,
			Content: step.Content, ToolCalls: step.ToolCalls, Usage: step.Usage,
			FinishReason: step.FinishReason, RawFinishReason: step.RawFinishReason,
			ProviderMetadata: step.ProviderMetadata, Warnings: step.Warnings,
			Sources: step.Sources, Files: step.Files,
		}
		for _, result := range step.ToolResults {
			out[i].ToolResults = append(out[i].ToolResults, result.ID+"|"+result.Name+"|"+result.Output)
		}
	}
	return out
}

// The aggregate a streamed run exposes must be the one Run returns, or
// StreamRun is not a replacement for it. The fixture uses no Streaming-sensitive
// hook, which is the one documented reason the two paths may legitimately
// differ.
func TestStreamRunResultEqualsRun(t *testing.T) {
	runModel := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
	ran, err := mustRunnerAgent(t, runModel).Runner().Prompt("question").Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	streamModel := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
	stream, err := mustRunnerAgent(t, streamModel).Runner().Prompt("question").StreamRun(context.Background())
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}
	for _, eventErr := range stream.Events() {
		if eventErr != nil {
			t.Fatalf("Events() error = %v", eventErr)
		}
	}
	streamed, err := stream.Result()
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}

	// aikit.ToolResult carries a typed Error, and reflect.DeepEqual on errors is
	// unreliable, so the aggregate is compared field by field. This fixture
	// produces no tool results; their count is asserted rather than their
	// contents.
	if !reflect.DeepEqual(streamed.Transcript, ran.Transcript) {
		t.Errorf("Transcript = %#v, want %#v", streamed.Transcript, ran.Transcript)
	}
	if !reflect.DeepEqual(comparableSteps(streamed.Steps), comparableSteps(ran.Steps)) {
		t.Errorf("Steps = %#v, want %#v", streamed.Steps, ran.Steps)
	}
	if !reflect.DeepEqual(streamed.Usage, ran.Usage) {
		t.Errorf("Usage = %#v, want %#v", streamed.Usage, ran.Usage)
	}
	if !reflect.DeepEqual(streamed.Sources, ran.Sources) {
		t.Errorf("Sources = %#v, want %#v", streamed.Sources, ran.Sources)
	}
	if !reflect.DeepEqual(streamed.Files, ran.Files) {
		t.Errorf("Files = %#v, want %#v", streamed.Files, ran.Files)
	}
	if !reflect.DeepEqual(streamed.Warnings, ran.Warnings) {
		t.Errorf("Warnings = %#v, want %#v", streamed.Warnings, ran.Warnings)
	}
	if !reflect.DeepEqual(streamed.ProviderMetadata, ran.ProviderMetadata) {
		t.Errorf("ProviderMetadata = %#v, want %#v", streamed.ProviderMetadata, ran.ProviderMetadata)
	}
	if streamed.MessageID != ran.MessageID || streamed.Text != ran.Text ||
		streamed.Reasoning != ran.Reasoning || streamed.FinishReason != ran.FinishReason ||
		streamed.RawFinishReason != ran.RawFinishReason ||
		len(streamed.ToolResults) != len(ran.ToolResults) ||
		len(streamed.PendingApprovals) != len(ran.PendingApprovals) ||
		string(streamed.StructuredOutput) != string(ran.StructuredOutput) {
		t.Errorf("scalar aggregate fields diverge:\nstreamed %#v\nran      %#v", streamed, ran)
	}
	if streamed.Text != "the answer" || streamed.Reasoning != "weighing" {
		t.Errorf("Text/Reasoning = (%q, %q)", streamed.Text, streamed.Reasoning)
	}
	if len(streamed.Sources) != 1 || len(streamed.Files) != 1 || len(streamed.Warnings) != 1 {
		t.Errorf("Sources/Files/Warnings = (%d, %d, %d), want (1, 1, 1)",
			len(streamed.Sources), len(streamed.Files), len(streamed.Warnings))
	}
	if len(streamed.Transcript) != 2 {
		t.Errorf("Transcript len = %d, want 2 (prompt plus the assistant turn)", len(streamed.Transcript))
	}
}

// The structured-output finishing call is an extra provider call on BOTH paths.
// Counting per-path rather than per-step is the only correct comparison.
func TestStreamRunMakesTheSameProviderCallsAsRun(t *testing.T) {
	script := [][]aikit.StreamEvent{richRunScript()}

	runModel := &runnerScriptModel{scripts: script}
	if _, err := mustRunnerAgent(t, runModel).Runner().Prompt("q").Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	streamModel := &runnerScriptModel{scripts: script}
	stream, err := mustRunnerAgent(t, streamModel).Runner().Prompt("q").StreamRun(context.Background())
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}
	for range stream.Events() {
	}

	runModel.mu.Lock()
	runCalls := len(runModel.requests)
	runModel.mu.Unlock()
	streamModel.mu.Lock()
	streamCalls := len(streamModel.requests)
	streamModel.mu.Unlock()

	if runCalls != streamCalls {
		t.Fatalf("provider calls = %d streamed, %d run, want equal", streamCalls, runCalls)
	}
}

func TestStreamRunRejectsResultBeforeDrain(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
	stream, err := mustRunnerAgent(t, model).Runner().Prompt("q").StreamRun(context.Background())
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}
	if stream.State() != agent.StreamNotDrained {
		t.Errorf("State() = %v, want StreamNotDrained", stream.State())
	}
	result, err := stream.Result()
	if !errors.Is(err, agent.ErrStreamNotDrained) {
		t.Fatalf("Result() error = %v, want ErrStreamNotDrained", err)
	}
	if result != nil {
		t.Errorf("Result() = %#v, want nil before drain", result)
	}
}

// Breaking on StepEventDone is a normal early stop with a whole aggregate.
// Reporting it as context.Canceled would make Result lie about a run that
// succeeded.
func TestStreamRunBreakOnDoneCompletesWithoutSynthesizedCancellation(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
	stream, err := mustRunnerAgent(t, model).Runner().Prompt("q").StreamRun(context.Background())
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}
	for event, eventErr := range stream.Events() {
		if eventErr != nil {
			t.Fatalf("Events() error = %v", eventErr)
		}
		if event.Type == aikit.StepEventDone {
			break
		}
	}

	if stream.State() != agent.StreamCompleted {
		t.Errorf("State() = %v, want StreamCompleted", stream.State())
	}
	result, err := stream.Result()
	if err != nil {
		t.Fatalf("Result() error = %v, want nil — no cancellation happened", err)
	}
	if result.Text != "the answer" || result.FinishReason != aikit.FinishReasonStop {
		t.Errorf("result = %#v, want the complete aggregate", result)
	}
}

func TestStreamRunBreakBeforeDoneAborts(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
	stream, err := mustRunnerAgent(t, model).Runner().Prompt("q").StreamRun(context.Background())
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}
	for event := range stream.Events() {
		if event.Type == aikit.StepEventTextDelta {
			break
		}
	}

	if stream.State() != agent.StreamAborted {
		t.Errorf("State() = %v, want StreamAborted", stream.State())
	}
	result, err := stream.Result()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Result() error = %v, want context.Canceled", err)
	}
	if result == nil {
		t.Fatal("Result() = nil with a non-nil error, want the partial aggregate")
	}
}

func TestStreamRunIsSingleUseAndKeepsTheFirstRangeAggregate(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
	stream, err := mustRunnerAgent(t, model).Runner().Prompt("q").StreamRun(context.Background())
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}
	for range stream.Events() {
	}
	first, err := stream.Result()
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}

	var secondErr error
	var count int
	for _, eventErr := range stream.Events() {
		count++
		secondErr = eventErr
	}
	if count != 1 || !errors.Is(secondErr, agent.ErrStreamUsed) {
		t.Fatalf("second range = (%d events, %v), want one ErrStreamUsed", count, secondErr)
	}

	second, err := stream.Result()
	if err != nil {
		t.Fatalf("Result() after a second range error = %v", err)
	}
	if second != first {
		t.Error("Result() changed after a rejected second range")
	}
}

func TestStreamRunMaxTurnsReturnsPartialResultAndError(t *testing.T) {
	toolCall := []aikit.StreamEvent{
		{
			Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0,
			ToolCallID: "tc1", ToolCallName: "echo", ToolCallArgsDelta: `{"value":"x"}`,
		},
		{Type: aikit.StreamEventFinish, MessageID: "m1", FinishReason: aikit.FinishReasonToolCalls},
	}
	echo, err := tool.NewDynamic(
		"echo",
		"test echo",
		map[string]any{"type": "object"},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"echoed":"x"}`), nil
		},
	)
	if err != nil {
		t.Fatalf("NewDynamic() error = %v", err)
	}
	tools, err := tool.NewSet(echo)
	if err != nil {
		t.Fatalf("NewSet() error = %v", err)
	}
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{toolCall, toolCall}}
	built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
		return builder.Tools(tools).MaxTurns(1)
	})

	stream, err := built.Runner().Prompt("q").StreamRun(context.Background())
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}
	var streamErr error
	for _, eventErr := range stream.Events() {
		if eventErr != nil {
			streamErr = eventErr
		}
	}

	var maxTurns *agent.MaxTurnsError
	if !errors.As(streamErr, &maxTurns) {
		t.Fatalf("Events() error = %v, want *MaxTurnsError", streamErr)
	}
	result, err := stream.Result()
	if !errors.As(err, &maxTurns) {
		t.Fatalf("Result() error = %v, want *MaxTurnsError", err)
	}
	if result == nil {
		t.Fatal("Result() = nil, want the partial result")
	}
	if maxTurns.Result != result {
		t.Error("MaxTurnsError.Result is not the stream's aggregate")
	}
	if len(result.Transcript) == 0 {
		t.Error("Transcript is empty, want the turns that did run")
	}
}

// RunFinishedHook observes the same terminal error Result reports, and it must
// run before the consumer's range returns so Result is readable straight after.
func TestStreamRunFinishedHookMatchesTheTerminalError(t *testing.T) {
	cases := []struct {
		name      string
		breakOn   aikit.StepEventType
		wantErr   error
		wantState agent.StreamState
	}{
		{
			name:      "break on done",
			breakOn:   aikit.StepEventDone,
			wantErr:   nil,
			wantState: agent.StreamCompleted,
		},
		{
			name:      "break before done",
			breakOn:   aikit.StepEventTextDelta,
			wantErr:   context.Canceled,
			wantState: agent.StreamAborted,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var hookErr error
			var hookRan bool
			model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
			built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
				return builder.Hook(agent.HookFuncs{
					Name: "terminal-error-observer",
					RunFinishedFunc: func(_ context.Context, _ agent.HookContext, _ *agent.Result, err error) {
						hookRan, hookErr = true, err
					},
				})
			})

			stream, err := built.Runner().Prompt("q").StreamRun(context.Background())
			if err != nil {
				t.Fatalf("StreamRun() error = %v", err)
			}
			for event := range stream.Events() {
				if event.Type == test.breakOn {
					break
				}
			}

			if !hookRan {
				t.Fatal("RunFinishedHook did not run before the range returned")
			}
			if !errors.Is(hookErr, test.wantErr) {
				t.Errorf("hook error = %v, want %v", hookErr, test.wantErr)
			}
			if stream.State() != test.wantState {
				t.Errorf("State() = %v, want %v", stream.State(), test.wantState)
			}
			_, resultErr := stream.Result()
			if !errors.Is(resultErr, test.wantErr) {
				t.Errorf("Result() error = %v, want %v — it must match what the hook saw", resultErr, test.wantErr)
			}
		})
	}
}

// Agent is the second implementer the aikit streaming interfaces exist for.
func TestAgentSatisfiesTheStreamingInterfacesOverStepEvents(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
	built := mustRunnerAgent(t, model)

	var prompter aikit.StreamingPrompt[aikit.StepEvent, *agent.StepStream] = built
	stream, err := prompter.StreamPrompt(context.Background(), "q")
	if err != nil {
		t.Fatalf("StreamPrompt() error = %v", err)
	}
	for range stream.Events() {
	}
	result, err := stream.Result()
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if result.Text != "the answer" {
		t.Errorf("Text = %q, want the answer", result.Text)
	}
}

func TestAgentStreamChatPlacesHistoryBeforeThePrompt(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
	built := mustRunnerAgent(t, model)
	history := []aikit.Message{aikit.UserMessage("earlier"), aikit.AssistantMessage("noted")}

	stream, err := built.StreamChat(context.Background(), "now", history...)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	for range stream.Events() {
	}

	model.mu.Lock()
	request := model.requests[0]
	model.mu.Unlock()
	if len(request.Messages) != 3 {
		t.Fatalf("request messages = %d, want 3", len(request.Messages))
	}
	if request.Messages[0].Content[0].Text != "earlier" ||
		request.Messages[2].Content[0].Text != "now" {
		t.Fatalf("request messages = %#v", request.Messages)
	}
}

func TestAgentStreamCompletionReturnsAShapeableRunner(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
	built := mustRunnerAgent(t, model)

	runner, err := built.StreamCompletion(context.Background(), "now")
	if err != nil {
		t.Fatalf("StreamCompletion() error = %v", err)
	}
	stream, err := runner.MaxTurns(3).StreamRun(context.Background())
	if err != nil {
		t.Fatalf("StreamRun() error = %v", err)
	}
	for range stream.Events() {
	}
	if _, err := stream.Result(); err != nil {
		t.Fatalf("Result() error = %v", err)
	}
}

// A forwarder that returned a nil pointer boxed in an interface would pass a
// `!= nil` check and panic on first use.
func TestAgentStreamingForwardersReturnGenuineNilOnValidationError(t *testing.T) {
	var built *agent.Agent

	t.Run("StreamPrompt", func(t *testing.T) {
		stream, err := built.StreamPrompt(context.Background(), "q")
		if err == nil {
			t.Fatal("error = nil for a nil agent")
		}
		if stream != nil {
			t.Fatalf("stream = %#v, want a nil *StepStream", stream)
		}
	})
	t.Run("StreamChat", func(t *testing.T) {
		stream, err := built.StreamChat(context.Background(), "q")
		if err == nil {
			t.Fatal("error = nil for a nil agent")
		}
		if stream != nil {
			t.Fatalf("stream = %#v, want a nil *StepStream", stream)
		}
	})
	t.Run("empty prompt", func(t *testing.T) {
		model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}}
		stream, err := mustRunnerAgent(t, model).StreamPrompt(context.Background(), "")
		if err == nil {
			t.Fatal("error = nil for an empty prompt")
		}
		if stream != nil {
			t.Fatalf("stream = %#v, want a nil *StepStream", stream)
		}
	})
}
