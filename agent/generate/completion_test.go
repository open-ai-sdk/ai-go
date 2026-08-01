package generate

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

type agentCompletionModel struct {
	requests []llm.Request
	calls    int
}

func (*agentCompletionModel) ModelID() string { return "agent-completion" }

func (m *agentCompletionModel) Stream(_ context.Context, request llm.Request) (<-chan aikit.StreamEvent, error) {
	m.requests = append(m.requests, request)
	m.calls++
	ch := make(chan aikit.StreamEvent, 2)
	if m.calls == 1 && len(request.Tools) > 0 {
		ch <- aikit.StreamEvent{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "call-1", ToolCallName: "lookup", ToolCallArgsDelta: `{}`}
		ch <- aikit.StreamEvent{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonToolCalls}
	} else {
		ch <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: "done"}
		ch <- aikit.StreamEvent{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonStop}
	}
	close(ch)
	return ch, nil
}

type agentCompletionExecutor struct{ calls int32 }

func (e *agentCompletionExecutor) Execute(context.Context, string, string) (string, error) {
	atomic.AddInt32(&e.calls, 1)
	return `{"ok":true}`, nil
}

func TestToolLoopAgentCompletionUsesAgentDefaultsAndTools(t *testing.T) {
	model := &agentCompletionModel{}
	executor := &agentCompletionExecutor{}
	tools := &ToolSet{Definitions: []ToolDefinition{{Name: "lookup"}}, Executor: executor}
	agent := NewToolLoopAgent(model, WithAgentInstructions("default"), WithAgentTools(tools))

	result, err := agent.Completion("weather").Instructions("override").Temperature(0.2).Send(context.Background())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.Text != "done" || model.calls != 2 || atomic.LoadInt32(&executor.calls) != 1 {
		t.Fatalf("result=%#v model.calls=%d executor.calls=%d", result, model.calls, executor.calls)
	}
	if len(model.requests[0].Messages) != 2 || model.requests[0].Messages[0].Role != RoleSystem || model.requests[0].Messages[0].Content[0].Text != "override" || model.requests[0].Settings.Temperature == nil || *model.requests[0].Settings.Temperature != 0.2 || model.requests[0].Messages[1].Content[0].Text != "weather" {
		t.Fatalf("first request = %#v", model.requests[0])
	}
}

func TestToolLoopAgentCompletionPromptAndChat(t *testing.T) {
	model := &agentCompletionModel{calls: 1}
	agent := NewToolLoopAgent(model)
	text, err := agent.Prompt(context.Background(), "prompt")
	if err != nil || text != "done" {
		t.Fatalf("Prompt text=%q err=%v", text, err)
	}

	text, err = agent.Chat(context.Background(), "next", UserMessage("history"))
	if err != nil || text != "done" || len(model.requests[1].Messages) != 2 || model.requests[1].Messages[1].Content[0].Text != "next" {
		t.Fatalf("Chat text=%q err=%v request=%#v", text, err, model.requests[1])
	}
}

func TestToolLoopAgentImplementsCompletion(t *testing.T) {
	var _ Completion = NewToolLoopAgent(&agentCompletionModel{calls: 1})
}
