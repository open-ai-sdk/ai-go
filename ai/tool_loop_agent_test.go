package ai_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/ai"
)

// minimalAgent is a from-scratch Agent implementation built only from
// exported ai package types — it never touches ToolLoopAgent. Its existence
// (and the var _ ai.Agent assertion below) is the actual proof that a
// consumer can implement Agent themselves; it is not merely aspirational.
type minimalAgent struct {
	tools *ai.ToolSet
}

var _ ai.Agent = (*minimalAgent)(nil)

func (m *minimalAgent) ID() string         { return "minimal" }
func (m *minimalAgent) Tools() *ai.ToolSet { return m.tools }
func (m *minimalAgent) Generate(ctx context.Context, opts ...ai.Option) (*ai.GenerateTextResult, error) {
	req := ai.GenerateTextRequest{Tools: m.tools}
	for _, o := range opts {
		o(&req)
	}
	return ai.GenerateText(ctx, req)
}

func (m *minimalAgent) Stream(ctx context.Context, opts ...ai.Option) (*ai.StreamResult, error) {
	req := ai.GenerateTextRequest{Tools: m.tools}
	for _, o := range opts {
		o(&req)
	}
	return ai.StreamText(ctx, req), nil
}

// TestToolLoopAgent_ThreeStepFakeExecutor verifies ToolLoopAgent delegates to
// the tool loop by running fakeToolExecutor and fakeMultiStepModel through
// three steps.
func TestToolLoopAgent_ThreeStepFakeExecutor(t *testing.T) {
	model := &fakeMultiStepModel{}
	executor := &fakeToolExecutor{}

	agent := ai.NewToolLoopAgent(model, ai.WithAgentTools(&ai.ToolSet{
		Definitions: []ai.ToolDefinition{
			{Name: "lookup", InputSchema: map[string]any{"type": "object"}},
			{Name: "noop", InputSchema: map[string]any{"type": "object"}},
		},
		Executor: executor,
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := agent.Generate(ctx, ai.WithMessages(ai.UserMessage("resolve a then noop")))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Text != "done" {
		t.Errorf("Text = %q, want %q", result.Text, "done")
	}
	if len(result.Steps) != 3 {
		t.Fatalf("Steps = %d, want 3", len(result.Steps))
	}
	if got := atomic.LoadInt32(&executor.calls); got != 2 {
		t.Errorf("executor.calls = %d, want 2", got)
	}
}

// alwaysToolCallModel emits a tool call on every step, regardless of call
// count, so a stop condition (not the model running dry) is what ends the run.
type alwaysToolCallModel struct{ calls int32 }

func (m *alwaysToolCallModel) ModelID() string { return "always-tool-call" }

func (m *alwaysToolCallModel) Stream(
	_ context.Context,
	_ ai.LanguageModelRequest,
) (<-chan ai.StreamEvent, error) {
	n := atomic.AddInt32(&m.calls, 1)
	ch := make(chan ai.StreamEvent, 2)
	ch <- ai.StreamEvent{
		Type:              ai.StreamEventToolCallDelta,
		ToolCallID:        fmt.Sprintf("tc-%d", n),
		ToolCallName:      "noop",
		ToolCallArgsDelta: `{}`,
	}
	ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonToolCalls}
	close(ch)
	return ch, nil
}

type noopExecutor struct{}

func (noopExecutor) Execute(_ context.Context, _, _ string) (string, error) {
	return `{"ok":true}`, nil
}

func newNoopAgent(model ai.LanguageModel, opts ...ai.AgentOption) *ai.ToolLoopAgent {
	base := []ai.AgentOption{ai.WithAgentTools(&ai.ToolSet{
		Definitions: []ai.ToolDefinition{{Name: "noop", InputSchema: map[string]any{"type": "object"}}},
		Executor:    noopExecutor{},
	})}
	return ai.NewToolLoopAgent(model, append(base, opts...)...)
}

// TestToolLoopAgent_DefaultStopWhenRunsPastTenAndStopsAtTwenty verifies an
// agent with the default stop condition runs a full 20 steps — past what
// would have been the engine's old implicit ten-step ceiling — and stops
// there (ToolLoopAgent's default, IsStepCount(20)), proving the
// engine's loop bound is no longer an implicit cap independent of StopWhen.
func TestToolLoopAgent_DefaultStopWhenRunsPastTenAndStopsAtTwenty(t *testing.T) {
	model := &alwaysToolCallModel{}
	agent := newNoopAgent(model)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := agent.Generate(ctx, ai.WithMessages(ai.UserMessage("go")))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(result.Steps) != 20 {
		t.Fatalf("Steps = %d, want 20 (default stopWhen=IsStepCount(20))", len(result.Steps))
	}
	if got := atomic.LoadInt32(&model.calls); got != 20 {
		t.Errorf("model called %d times, want 20", got)
	}
}

// TestToolLoopAgent_CallbackMerge_AgentAndCallBothFire is an integration-level
// companion to the direct mergeCallback unit tests: it proves a real
// Generate() call invokes both the agent-level and the call-level OnStepEnd
// for every step, and that a panicking agent-level callback does not stop the
// call-level one or fail the run.
func TestToolLoopAgent_CallbackMerge_AgentAndCallBothFire(t *testing.T) {
	var agentCalls, callCalls int32
	agent := newNoopAgent(&alwaysToolCallModel{}, ai.WithAgentOnStepEnd(func(ai.StepEndEvent) {
		atomic.AddInt32(&agentCalls, 1)
		panic("agent-level callback misbehaving")
	}), ai.WithAgentStopWhen(ai.IsStepCount(3)))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := agent.Generate(ctx,
		ai.WithMessages(ai.UserMessage("go")),
		ai.WithOnStepEnd(func(ai.StepEndEvent) { atomic.AddInt32(&callCalls, 1) }),
	)
	if err != nil {
		t.Fatalf("Generate must not fail because a callback panicked: %v", err)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("Steps = %d, want 3", len(result.Steps))
	}
	if got := atomic.LoadInt32(&agentCalls); got != 3 {
		t.Errorf("agent-level OnStepEnd fired %d times, want 3", got)
	}
	if got := atomic.LoadInt32(&callCalls); got != 3 {
		t.Errorf("call-level OnStepEnd fired %d times, want 3", got)
	}
}

// TestToolLoopAgent_PerCallOptionOverridesAgentDefault verifies a per-call
// Option replaces the agent's scalar default (StopWhen) for that call only.
func TestToolLoopAgent_PerCallOptionOverridesAgentDefault(t *testing.T) {
	agent := newNoopAgent(&alwaysToolCallModel{}, ai.WithAgentStopWhen(ai.IsStepCount(20)))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := agent.Generate(ctx,
		ai.WithMessages(ai.UserMessage("go")),
		ai.WithStopWhen(ai.IsStepCount(2)),
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("Steps = %d, want 2 (per-call StopWhen must override the agent default)", len(result.Steps))
	}
}

// TestToolLoopAgent_ToolsReflectsConstructionDefault verifies Tools() and
// ID() surface the values configured via AgentOption.
func TestToolLoopAgent_ToolsReflectsConstructionDefault(t *testing.T) {
	ts := &ai.ToolSet{
		Definitions: []ai.ToolDefinition{{Name: "noop", InputSchema: map[string]any{"type": "object"}}},
		Executor:    noopExecutor{},
	}
	agent := ai.NewToolLoopAgent(&alwaysToolCallModel{}, ai.WithAgentID("agent-1"), ai.WithAgentTools(ts))

	if agent.ID() != "agent-1" {
		t.Errorf("ID() = %q, want %q", agent.ID(), "agent-1")
	}
	if agent.Tools() != ts {
		t.Error("Tools() did not return the configured ToolSet")
	}
}
