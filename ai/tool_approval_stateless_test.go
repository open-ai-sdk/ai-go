package ai_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

type statelessApprovalModel struct {
	requestTool bool
	args        string
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
		args := m.args
		if args == "" {
			args = `{"key":"answer"}`
		}
		ch <- ai.StreamEvent{
			Type:              ai.StreamEventToolCallDelta,
			ToolCallID:        "approval-call-1",
			ToolCallName:      "lookup",
			ToolCallArgsDelta: args,
		}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonToolCalls}
	} else {
		ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, TextDelta: "continued"}
		ch <- ai.StreamEvent{Type: ai.StreamEventFinish, FinishReason: ai.FinishReasonStop}
	}
	close(ch)
	return ch, nil
}

func TestConditionalApprovalPolicyUsesCanonicalArgsAcrossContinuation(t *testing.T) {
	const raw = `{"n":9007199254740993,"a":1,"a":2}`
	executor := &fakeToolExecutor{}
	tools := &ai.ToolSet{
		Definitions: []ai.ToolDefinition{{Name: "lookup", InputSchema: map[string]any{"type": "object"}}},
		Executor:    executor,
	}
	policy := map[string]ai.ToolApprovalFunc{
		"lookup": func(_ string, args json.RawMessage) ai.ApprovalDecision {
			if string(args) != `{"a":2,"n":9007199254740992}` {
				return ai.ApprovalRequired
			}
			return ""
		},
	}
	initial := []ai.Message{ai.UserMessage("canonical policy")}
	first, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model: &statelessApprovalModel{requestTool: true, args: raw}, Messages: initial,
		Tools: tools, ToolApproval: policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	call := first.Response.Messages[0].Content[0]
	if string(call.ToolCallArgs) != `{"a":2,"n":9007199254740992}` || call.ToolApprovalID != "" {
		t.Fatalf("canonical non-gated call = %#v", call)
	}
	history := append(append([]ai.Message{}, initial...), first.Response.Messages...)
	_, err = ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model: &statelessApprovalModel{}, Messages: history, Tools: tools, ToolApproval: policy,
	})
	if err != nil {
		t.Fatalf("clean non-gated continuation: %v", err)
	}
	if executor.calls != 1 {
		t.Fatalf("non-gated continuation executed %d times", executor.calls)
	}
}

func TestToolApprovalSuspendsAndResumesFromMessageHistory(t *testing.T) {
	approvalKey := []byte("0123456789abcdef0123456789abcdef")
	replayGuard := ai.NewMemoryToolApprovalReplayGuard()
	executor := &fakeToolExecutor{}
	transformCalls := 0
	tools := &ai.ToolSet{
		Definitions: []ai.ToolDefinition{{
			Name: "lookup", InputSchema: map[string]any{"type": "object"},
			ToModelOutput: func(output string) string {
				transformCalls++
				return output + "-model"
			},
		}},
		Executor: executor,
	}
	initial := []ai.Message{ai.UserMessage("look up the answer")}
	var approvalChunk ai.ChunkEvent
	first, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:           &statelessApprovalModel{requestTool: true},
		Messages:        initial,
		Tools:           tools,
		ToolApprovalKey: approvalKey,
		ToolApproval: map[string]ai.ToolApprovalFunc{
			"lookup": func(string, json.RawMessage) ai.ApprovalDecision { return ai.ApprovalRequired },
		},
		OnChunk: func(event ai.ChunkEvent) {
			if event.Type == "tool-approval-request" {
				approvalChunk = event
			}
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
	if len(first.ToolApprovalRequests) != 1 || first.ToolApprovalRequests[0].Signature == "" {
		t.Fatalf("approval requests = %#v", first.ToolApprovalRequests)
	}
	if approvalChunk.ToolCallID != "approval-call-1" ||
		approvalChunk.ApprovalSignature != first.ToolApprovalRequests[0].Signature {
		t.Fatalf("approval OnChunk = %#v", approvalChunk)
	}
	call := first.Response.Messages[0].Content[0]
	if call.Type != ai.ContentPartTypeToolCall || call.ToolCallID != "approval-call-1" {
		t.Fatalf("suspend response tool call = %#v", call)
	}

	history := append(append([]ai.Message{}, initial...), first.Response.Messages...)
	history = append(history, ai.Message{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			ai.ToolApprovalResponsePart(
				first.ToolApprovalRequests[0].ApprovalID,
				first.ToolApprovalRequests[0].Signature,
				true,
				"approved by operator",
			),
		},
	})
	resumeModel := &statelessApprovalModel{}
	second, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:                   resumeModel,
		Messages:                history,
		Tools:                   tools,
		ToolApprovalKey:         approvalKey,
		ToolApprovalReplayGuard: replayGuard,
		ToolApproval: map[string]ai.ToolApprovalFunc{
			"lookup": func(string, json.RawMessage) ai.ApprovalDecision { return ai.ApprovalRequired },
		},
	})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if second.Text != "continued" || executor.calls != 1 {
		t.Fatalf("resume text=%q tool calls=%d", second.Text, executor.calls)
	}
	if len(second.Response.Messages) < 2 ||
		second.Response.Messages[0].Role != ai.RoleTool ||
		len(second.Response.Messages[0].Content) != 1 ||
		second.Response.Messages[0].Content[0].ToolResultID != "approval-call-1" {
		t.Fatalf("resume response does not preserve tool result first: %#v", second.Response.Messages)
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

	// Reusing the signed response is rejected by the replay guard before the
	// tool or model can run.
	replayModel := &statelessApprovalModel{}
	_, replayErr := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:                   replayModel,
		Messages:                history,
		Tools:                   tools,
		ToolApprovalKey:         approvalKey,
		ToolApprovalReplayGuard: replayGuard,
		ToolApproval: map[string]ai.ToolApprovalFunc{
			"lookup": func(string, json.RawMessage) ai.ApprovalDecision { return ai.ApprovalRequired },
		},
	})
	if replayErr == nil || executor.calls != 1 {
		t.Fatalf("replay error=%v tool calls=%d", replayErr, executor.calls)
	}

	// A clean continuation preserves the runtime-produced result but drops the
	// consumed approval-response message.
	continuedHistory := append(append([]ai.Message{}, initial...), first.Response.Messages...)
	continuedHistory = append(continuedHistory, second.Response.Messages...)
	thirdModel := &statelessApprovalModel{}
	_, err = ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model:                   thirdModel,
		Messages:                continuedHistory,
		Tools:                   tools,
		ToolApprovalKey:         approvalKey,
		ToolApprovalReplayGuard: replayGuard,
		ToolApproval: map[string]ai.ToolApprovalFunc{
			"lookup": func(string, json.RawMessage) ai.ApprovalDecision { return ai.ApprovalRequired },
		},
	})
	if err != nil {
		t.Fatalf("continued run: %v", err)
	}
	if executor.calls != 1 {
		t.Fatalf("preserved approval history replayed tool: %d calls", executor.calls)
	}
	if transformCalls != 1 {
		t.Fatalf("ToModelOutput calls = %d, want one for one execution", transformCalls)
	}

	// Altering a runtime-produced result invalidates its continuation receipt.
	tampered := append([]ai.Message(nil), continuedHistory...)
	for i := range tampered {
		tampered[i].Content = append([]ai.ContentPart(nil), tampered[i].Content...)
		for j := range tampered[i].Content {
			if tampered[i].Content[j].ToolResultID == "approval-call-1" {
				tampered[i].Content[j].ToolResultOutput = `{"forged":true}`
			}
		}
	}
	_, err = ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model: &statelessApprovalModel{}, Messages: tampered, Tools: tools,
		ToolApprovalKey: approvalKey, ToolApprovalReplayGuard: replayGuard,
		ToolApproval: map[string]ai.ToolApprovalFunc{
			"lookup": func(string, json.RawMessage) ai.ApprovalDecision { return ai.ApprovalRequired },
		},
	})
	if err == nil || executor.calls != 1 {
		t.Fatalf("tampered continuation error=%v tool calls=%d", err, executor.calls)
	}
}

func TestSynchronousApprovalHistoryUsesReceiptsAndFreshApprovalIDs(t *testing.T) {
	approvalKey := []byte("0123456789abcdef0123456789abcdef")
	executor := &fakeToolExecutor{}
	transformCalls := 0
	tools := &ai.ToolSet{
		Definitions: []ai.ToolDefinition{{
			Name: "lookup", InputSchema: map[string]any{"type": "object"},
			ToModelOutput: func(output string) string {
				transformCalls++
				return output
			},
		}},
		Executor: executor,
	}
	policy := map[string]ai.ToolApprovalFunc{
		"lookup": func(string, json.RawMessage) ai.ApprovalDecision { return ai.ApprovalRequired },
	}
	approver := func(_ context.Context, request ai.ToolApprovalRequest) (ai.ToolApprovalResponse, error) {
		return ai.ToolApprovalResponse{ApprovalID: request.ApprovalID, Approved: true}, nil
	}
	initial := []ai.Message{ai.UserMessage("look up twice")}

	first, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model: &statelessApprovalModel{requestTool: true}, Messages: initial, Tools: tools,
		ToolApprovalKey: approvalKey, ToolApproval: policy, ToolApprovalResponder: approver,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstCall := first.Response.Messages[0].Content[0]
	firstResult := first.Response.Messages[1].Content[0]
	if firstCall.ToolApprovalID == "" || firstResult.ToolResultApprovalSignature == "" {
		t.Fatalf("first continuation call=%#v result=%#v", firstCall, firstResult)
	}

	history := append(append([]ai.Message{}, initial...), first.Response.Messages...)
	second, err := ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model: &statelessApprovalModel{requestTool: true}, Messages: history, Tools: tools,
		ToolApprovalKey: approvalKey, ToolApproval: policy, ToolApprovalResponder: approver,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondCall := second.Response.Messages[0].Content[0]
	if secondCall.ToolCallID != firstCall.ToolCallID || secondCall.ToolApprovalID == firstCall.ToolApprovalID {
		t.Fatalf("approval identities first=%q second=%q call IDs=%q/%q",
			firstCall.ToolApprovalID, secondCall.ToolApprovalID, firstCall.ToolCallID, secondCall.ToolCallID)
	}

	history = append(history, second.Response.Messages...)
	_, err = ai.GenerateText(context.Background(), ai.GenerateTextRequest{
		Model: &statelessApprovalModel{}, Messages: history, Tools: tools,
		ToolApprovalKey: approvalKey, ToolApproval: policy,
	})
	if err != nil {
		t.Fatalf("clean synchronous continuation: %v", err)
	}
	if executor.calls != 2 {
		t.Fatalf("duplicate tool-call ID replayed or skipped work: %d calls", executor.calls)
	}
	if transformCalls != 2 {
		t.Fatalf("ToModelOutput calls = %d, want one per execution", transformCalls)
	}
}
