package ai_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

type statelessApprovalModel struct {
	requestTool bool
	lastRequest ai.LanguageModelRequest
}

func (m *statelessApprovalModel) ModelID() string { return "stateless-approval" }

func (m *statelessApprovalModel) Stream(
	_ context.Context,
	req ai.LanguageModelRequest,
) (<-chan ai.StreamEvent, error) {
	m.lastRequest = req
	ch := make(chan ai.StreamEvent, 2)
	if m.requestTool {
		ch <- ai.StreamEvent{
			Type:              ai.StreamEventToolCallDelta,
			ToolCallID:        "approval-call-1",
			ToolCallName:      "lookup",
			ToolCallArgsDelta: `{"key":"answer"}`,
		}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonToolCalls}
	} else {
		ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, TextDelta: "continued"}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop}
	}
	close(ch)
	return ch, nil
}

func TestToolApprovalSuspendsAndResumesFromMessageHistory(t *testing.T) {
	executor := &fakeToolExecutor{}
	tools := &ai.ToolSet{
		Definitions: []ai.ToolDefinition{{Name: "lookup", InputSchema: map[string]any{"type": "object"}}},
		Executor:    executor,
	}
	initial := []ai.Message{ai.UserMessage("look up the answer")}
	first, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:    &statelessApprovalModel{requestTool: true},
		Messages: initial,
		Tools:    tools,
		ToolApproval: map[string]ai.ToolApprovalFunc{
			"lookup": func(string, json.RawMessage) ai.ApprovalDecision { return ai.ApprovalRequired },
		},
	})
	if err != nil {
		t.Fatalf("suspend run: %v", err)
	}
	if executor.calls != 0 {
		t.Fatalf("tool executed before approval: %d calls", executor.calls)
	}
	if len(first.Response.Messages) != 1 || len(first.Response.Messages[0].Content) != 1 {
		t.Fatalf("suspend response messages = %#v", first.Response.Messages)
	}
	call := first.Response.Messages[0].Content[0]
	if call.Type != ai.ContentPartTypeToolCall || call.ToolCallID != "approval-call-1" {
		t.Fatalf("suspend response tool call = %#v", call)
	}

	history := append(append([]ai.Message{}, initial...), first.Response.Messages...)
	history = append(history, ai.Message{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			ai.ToolApprovalResponsePart("approval-call-1", true, "approved by operator"),
		},
	})
	resumeModel := &statelessApprovalModel{}
	second, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:    resumeModel,
		Messages: history,
		Tools:    tools,
	})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if second.Text != "continued" || executor.calls != 1 {
		t.Fatalf("resume text=%q tool calls=%d", second.Text, executor.calls)
	}
	var sawResult, sawApprovalResponse bool
	for _, message := range resumeModel.lastRequest.Messages {
		for _, part := range message.Content {
			sawResult = sawResult || part.Type == ai.ContentPartTypeToolResult && part.ToolResultID == "approval-call-1"
			sawApprovalResponse = sawApprovalResponse || part.Type == ai.ContentPartTypeToolApprovalResponse
		}
	}
	if !sawResult || sawApprovalResponse {
		t.Fatalf("provider history result=%v approval-response=%v", sawResult, sawApprovalResponse)
	}
}
