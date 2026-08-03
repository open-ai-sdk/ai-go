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
		ch <- aikit.StreamEvent{Type: aikit.StreamEventFinish, MessageID: "msg-tool", FinishReason: aikit.FinishReasonToolCalls}
	} else {
		ch <- aikit.StreamEvent{Type: aikit.StreamEventTextDelta, TextDelta: "done"}
		ch <- aikit.StreamEvent{Type: aikit.StreamEventFinish, MessageID: "msg-final", FinishReason: aikit.FinishReasonStop}
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
		t.Fatalf("result=%#v model.calls=%d executor.calls=%d", result, model.calls, atomic.LoadInt32(&executor.calls))
	}
	if result.MessageID != "msg-final" || len(result.Response.Messages) != 3 ||
		result.Response.Messages[0].ID != "msg-tool" || result.Response.Messages[2].ID != "msg-final" {
		t.Fatalf("message identity was not preserved: %#v", result)
	}
	if len(result.Transcript) != 5 || result.Transcript[0].Role != RoleSystem ||
		result.Transcript[1].Role != RoleUser || result.Transcript[4].ID != "msg-final" {
		t.Fatalf("full transcript was not preserved: %#v", result.Transcript)
	}
	if len(model.requests[0].Messages) != 2 || model.requests[0].Messages[0].Role != RoleSystem ||
		model.requests[0].Messages[0].Content[0].Text != "override" ||
		model.requests[0].Settings.Temperature == nil ||
		*model.requests[0].Settings.Temperature != 0.2 ||
		model.requests[0].Messages[1].Content[0].Text != "weather" {
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
	if err != nil || text != "done" || len(model.requests[1].Messages) != 2 ||
		model.requests[1].Messages[1].Content[0].Text != "next" {
		t.Fatalf("Chat text=%q err=%v request=%#v", text, err, model.requests[1])
	}
}

func TestToolLoopAgentImplementsCompletion(t *testing.T) {
	var _ Completion = NewToolLoopAgent(&agentCompletionModel{calls: 1})
}

func TestToolLoopAgentCompletionBuilderAppliesRequestShapingOptions(t *testing.T) {
	model := &agentCompletionModel{calls: 1}
	overrideModel := &agentCompletionModel{calls: 1}
	stop := IsStepCount(2)
	request := NewToolLoopAgent(model).Completion("weather").
		Model(overrideModel).
		TopP(0.8).
		TopK(40).
		Seed(7).
		StopSequences("END").
		MaxSteps(3).
		StopWhen(stop).
		ActiveTools("lookup").
		ToolsContext(ToolsContext{"lookup": map[string]any{"city": "Hanoi"}}).
		RuntimeContext(RuntimeContext{"requestID": "req-1"}).
		Options(WithMaxTokens(128)).
		Build()

	if request.Model != overrideModel || request.Settings.TopP == nil || *request.Settings.TopP != 0.8 ||
		request.Settings.TopK == nil ||
		*request.Settings.TopK != 40 ||
		request.Settings.Seed == nil ||
		*request.Settings.Seed != 7 ||
		request.Settings.MaxTokens != 128 ||
		len(request.Settings.StopSequences) != 1 ||
		request.Settings.StopSequences[0] != "END" ||
		request.MaxSteps != 3 ||
		request.StopWhen == nil ||
		len(request.ActiveTools) != 1 ||
		request.ActiveTools[0] != "lookup" {
		t.Fatalf("unexpected request settings: %#v", request)
	}
	if request.ToolsContext["lookup"].(map[string]any)["city"] != "Hanoi" ||
		request.RuntimeContext["requestID"] != "req-1" {
		t.Fatalf("unexpected request context: %#v %#v", request.ToolsContext, request.RuntimeContext)
	}
}

func TestToolLoopAgentCompletionOptionsCanReplaceMessages(t *testing.T) {
	model := &agentCompletionModel{calls: 1}
	_, err := NewToolLoopAgent(model).Completion("original").
		Options(WithMessages(UserMessage("replacement"))).
		Send(context.Background())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(model.requests) != 1 || len(model.requests[0].Messages) != 1 ||
		model.requests[0].Messages[0].Content[0].Text != "replacement" {
		t.Fatalf("unexpected request: %#v", model.requests)
	}
}
