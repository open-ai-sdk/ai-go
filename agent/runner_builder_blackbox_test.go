package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

type runnerScriptModel struct {
	mu       sync.Mutex
	scripts  [][]aikit.StreamEvent
	requests []llm.Request
}

func (m *runnerScriptModel) ModelID() string { return "runner-script" }

func (m *runnerScriptModel) Stream(_ context.Context, request llm.Request) (<-chan aikit.StreamEvent, error) {
	m.mu.Lock()
	call := len(m.requests)
	m.requests = append(m.requests, cloneRunnerRequest(request))
	if call >= len(m.scripts) {
		m.mu.Unlock()
		return nil, fmt.Errorf("unexpected model call %d", call+1)
	}
	events := append([]aikit.StreamEvent(nil), m.scripts[call]...)
	m.mu.Unlock()

	stream := make(chan aikit.StreamEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func (m *runnerScriptModel) requestSnapshots() []llm.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]llm.Request, len(m.requests))
	for i := range m.requests {
		requests[i] = cloneRunnerRequest(m.requests[i])
	}
	return requests
}

func cloneRunnerRequest(request llm.Request) llm.Request {
	request.Messages = cloneRunnerMessages(request.Messages)
	request.Tools = append([]aikit.ToolDefinition(nil), request.Tools...)
	if request.Output != nil {
		output := *request.Output
		request.Output = &output
	}
	return request
}

func cloneRunnerMessages(messages []aikit.Message) []aikit.Message {
	cloned := make([]aikit.Message, len(messages))
	for i := range messages {
		cloned[i] = messages[i].Clone()
	}
	return cloned
}

func runnerTextEvents(messageID, text string) []aikit.StreamEvent {
	return []aikit.StreamEvent{
		{Type: aikit.StreamEventTextDelta, TextDelta: text},
		{Type: aikit.StreamEventFinish, MessageID: messageID, FinishReason: aikit.FinishReasonStop},
	}
}

func mustRunnerAgent(t *testing.T, model llm.Model, configure ...func(agent.Builder) agent.Builder) *agent.Agent {
	t.Helper()
	builder := agent.New(model)
	for _, apply := range configure {
		builder = apply(builder)
	}
	built, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return built
}

func TestRunnerMessagesReplaceAndMessagePromptAppend(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{runnerTextEvents("answer-1", "ok")}}
	built := mustRunnerAgent(t, model)

	replaced := aikit.UserMessage("replacement")
	runner := built.Runner().
		Messages(aikit.UserMessage("discarded")).
		Message(aikit.AssistantMessage("also discarded")).
		Messages(replaced).
		Message(aikit.AssistantMessage("history answer")).
		Prompt("final prompt")

	// Messages and Message must take defensive copies at the fluent-call boundary.
	replaced.Content[0].Text = "mutated after Messages"
	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	requests := model.requestSnapshots()
	if len(requests) != 1 {
		t.Fatalf("model calls = %d, want 1", len(requests))
	}
	want := []aikit.Message{
		aikit.UserMessage("replacement"),
		aikit.AssistantMessage("history answer"),
		aikit.UserMessage("final prompt"),
	}
	if !reflect.DeepEqual(requests[0].Messages, want) {
		t.Fatalf("request messages = %#v, want %#v", requests[0].Messages, want)
	}
}

func TestRunnerInputValidationPreventsModelCall(t *testing.T) {
	tests := []struct {
		name   string
		runner func(*agent.Agent) agent.Runner
		field  string
	}{
		{
			name:   "no messages",
			runner: func(built *agent.Agent) agent.Runner { return built.Runner() },
			field:  "Messages",
		},
		{
			name:   "empty prompt",
			runner: func(built *agent.Agent) agent.Runner { return built.Runner().Prompt("") },
			field:  "Prompt",
		},
		{
			name: "invalid message",
			runner: func(built *agent.Agent) agent.Runner {
				return built.Runner().Message(aikit.Message{Role: aikit.RoleUser})
			},
			field: "Messages[0]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{runnerTextEvents("unused", "unused")}}
			built := mustRunnerAgent(t, model)
			result, err := test.runner(built).Run(context.Background())
			if result != nil {
				t.Fatalf("Run() result = %#v, want nil", result)
			}
			var runErr *agent.RunError
			if !errors.As(err, &runErr) || runErr.Field != test.field {
				t.Fatalf("Run() error = %#v, want *RunError field %q", err, test.field)
			}
			if calls := len(model.requestSnapshots()); calls != 0 {
				t.Fatalf("model calls = %d, want 0", calls)
			}
		})
	}
}

func TestRunnerRunAggregatesOneTurnResultAndTranscript(t *testing.T) {
	usage := &aikit.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{{
		{Type: aikit.StreamEventTextDelta, TextDelta: "hello "},
		{Type: aikit.StreamEventTextDelta, TextDelta: "world"},
		{Type: aikit.StreamEventUsage, Usage: usage},
		{Type: aikit.StreamEventFinish, MessageID: "assistant-1", FinishReason: aikit.FinishReasonStop},
	}}}
	built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
		return builder.Instructions("be concise")
	})

	result, err := built.Runner().Prompt("say hello").Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text != "hello world" || result.MessageID != "assistant-1" ||
		result.FinishReason != aikit.FinishReasonStop {
		t.Fatalf("result terminal fields = (%q, %q, %q)", result.Text, result.MessageID, result.FinishReason)
	}
	if result.Usage.TotalTokens != 5 || len(result.Steps) != 1 || result.FinalStep.Text != "hello world" {
		t.Fatalf("result aggregate = %#v", result)
	}
	wantTranscript := []aikit.Message{
		aikit.SystemMessage("be concise"),
		aikit.UserMessage("say hello"),
		{ID: "assistant-1", Role: aikit.RoleAssistant, Content: []aikit.ContentPart{aikit.TextPart("hello world")}},
	}
	if !reflect.DeepEqual(result.Transcript, wantTranscript) {
		t.Fatalf("transcript = %#v, want %#v", result.Transcript, wantTranscript)
	}
	if got := result.GeneratedMessages(); !reflect.DeepEqual(got, wantTranscript[2:]) {
		t.Fatalf("GeneratedMessages() = %#v, want %#v", got, wantTranscript[2:])
	}
}

func TestRunnerDefaultMaxTurnsReturnsPartialResultAfterToolContinuation(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{
		{
			{
				Type:              aikit.StreamEventToolCallDelta,
				ToolCallIndex:     0,
				ToolCallID:        "call-1",
				ToolCallName:      "lookup",
				ToolCallArgsDelta: `{"q":"go"}`,
			},
			{Type: aikit.StreamEventFinish, MessageID: "assistant-tool", FinishReason: aikit.FinishReasonToolCalls},
		},
	}}
	lookup, err := tool.NewDynamic(
		"lookup",
		"test lookup",
		map[string]any{"type": "object"},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"answer":"golang"}`), nil
		},
	)
	if err != nil {
		t.Fatalf("NewDynamic() error = %v", err)
	}
	tools, err := tool.NewSet(lookup)
	if err != nil {
		t.Fatalf("NewSet() error = %v", err)
	}
	built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder { return builder.Tools(tools) })
	if built.MaxTurns() != 1 {
		t.Fatalf("default MaxTurns() = %d, want 1", built.MaxTurns())
	}

	result, runErr := built.Runner().Prompt("look it up").Run(context.Background())
	var maxTurns *agent.MaxTurnsError
	if !errors.As(runErr, &maxTurns) {
		t.Fatalf("Run() error = %T %v, want *MaxTurnsError", runErr, runErr)
	}
	if maxTurns.MaxTurns != 1 || result == nil || maxTurns.Result == nil {
		t.Fatalf("MaxTurnsError = %#v, result = %#v", maxTurns, result)
	}
	if len(result.Steps) != 1 || len(result.ToolResults) != 1 || len(result.Transcript) != 3 {
		t.Fatalf("partial result = %#v", result)
	}
	if result.FinishReason != aikit.FinishReasonToolCalls || result.ToolResults[0].Output != `{"answer":"golang"}` {
		t.Fatalf("partial terminal/tool fields = %#v", result)
	}
	if len(model.requestSnapshots()) != 1 {
		t.Fatal("default budget allowed an unexpected continuation model call")
	}

	streamModel := &runnerScriptModel{scripts: model.scripts}
	streamAgent := mustRunnerAgent(
		t,
		streamModel,
		func(builder agent.Builder) agent.Builder { return builder.Tools(tools) },
	)
	sequence, err := streamAgent.Runner().Prompt("look it up").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var sawDone bool
	var streamErr error
	for event, err := range sequence {
		sawDone = sawDone || event.Type == aikit.StepEventDone
		if err != nil {
			streamErr = err
		}
	}
	if sawDone {
		t.Fatal("exhausted stream emitted Done")
	}
	if !errors.As(streamErr, &maxTurns) || maxTurns.Result == nil || len(maxTurns.Result.ToolResults) != 1 {
		t.Fatalf("stream error = %#v, want MaxTurnsError with partial result", streamErr)
	}
}

func TestRunnerStructuredOutputUsesFinalConstrainedTurn(t *testing.T) {
	output := llm.OutputSchema{Type: "object", Schema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"ok": map[string]any{"type": "boolean"}},
		"required":   []any{"ok"},
	}}
	oneTurnModel := &runnerScriptModel{scripts: [][]aikit.StreamEvent{runnerTextEvents("answer", `{"ok":true}`)}}
	oneTurn := mustRunnerAgent(
		t,
		oneTurnModel,
		func(builder agent.Builder) agent.Builder { return builder.Output(output) },
	)
	result, err := oneTurn.Runner().Prompt("answer as JSON").Run(context.Background())
	if err != nil || result == nil || string(result.StructuredOutput) != `{"ok":true}` {
		t.Fatalf("one-turn structured Run() = (%#v, %v)", result, err)
	}
	if len(oneTurnModel.requestSnapshots()) != 1 {
		t.Fatalf("one-turn model calls = %d, want 1", len(oneTurnModel.requestSnapshots()))
	}
	streamModel := &runnerScriptModel{scripts: [][]aikit.StreamEvent{runnerTextEvents("answer", `{"ok":true}`)}}
	streamAgent := mustRunnerAgent(
		t,
		streamModel,
		func(builder agent.Builder) agent.Builder { return builder.Output(output) },
	)
	sequence, err := streamAgent.Runner().Prompt("answer as JSON").Stream(context.Background())
	if err != nil {
		t.Fatalf("one-turn structured Stream() error = %v", err)
	}
	stepStarts := 0
	for event := range sequence {
		if event.Type == aikit.StepEventStepStart {
			stepStarts++
		}
	}
	if stepStarts != 1 {
		t.Fatalf("exhausted structured run emitted %d step starts, want 1", stepStarts)
	}

	twoTurnModel := &runnerScriptModel{scripts: [][]aikit.StreamEvent{runnerTextEvents("answer", `{"ok":true}`)}}
	var preparedTurns []int
	var hookTurns []int
	twoTurn := mustRunnerAgent(t, twoTurnModel, func(builder agent.Builder) agent.Builder {
		return builder.
			Output(output).
			MaxTurns(2).
			PrepareStep(func(info llm.PrepareStepContext) *llm.PrepareStepResult {
				preparedTurns = append(preparedTurns, info.StepNumber)
				return nil
			}).
			Hook(agent.HookFuncs{
				Name: "turn-counter",
				BeforeCompletionFunc: func(
					_ context.Context,
					hookContext agent.HookContext,
					_ llm.Request,
				) (agent.CompletionAction, error) {
					hookTurns = append(hookTurns, hookContext.Turn)
					return agent.CompletionAction{Kind: agent.CompletionContinue}, nil
				},
			})
	})
	result, err = twoTurn.Runner().Prompt("answer as JSON").Run(context.Background())
	if err != nil {
		t.Fatalf("two-turn structured Run() error = %v", err)
	}
	if string(result.StructuredOutput) != `{"ok":true}` || len(twoTurnModel.requestSnapshots()) != 1 {
		t.Fatalf("structured result/calls = (%s, %d)", result.StructuredOutput, len(twoTurnModel.requestSnapshots()))
	}
	if len(result.Steps) != 1 || !reflect.DeepEqual(preparedTurns, []int{0}) ||
		!reflect.DeepEqual(hookTurns, []int{1}) {
		t.Fatalf("structured lifecycle = steps:%d prepare:%v hooks:%v", len(result.Steps), preparedTurns, hookTurns)
	}
	requests := twoTurnModel.requestSnapshots()
	if requests[0].Output == nil || len(requests[0].Tools) != 0 {
		t.Fatalf("structured request = %#v", requests[0])
	}
}
