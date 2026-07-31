package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/open-ai-sdk/ai-go/tool"
)

// fixedApprover returns the same decision for every request.
type fixedApprover struct {
	approved bool
	reason   string
	requests []ApprovalRequest
}

func (a *fixedApprover) RequestApproval(_ context.Context, req ApprovalRequest) (ApprovalResponse, error) {
	a.requests = append(a.requests, req)
	return ApprovalResponse{ApprovalID: req.ApprovalID, Approved: a.approved, Reason: a.reason}, nil
}

// blockingApprover blocks until the context is cancelled, then reports the
// cancellation as an error. It models a decision that never arrives.
type blockingApprover struct{}

func (blockingApprover) RequestApproval(ctx context.Context, _ ApprovalRequest) (ApprovalResponse, error) {
	<-ctx.Done()
	return ApprovalResponse{}, ctx.Err()
}

// requireApproval builds a policy map that marks the named tool as requiring
// approval before it runs.
func requireApproval(tool string) map[string]func(string, string) bool {
	return map[string]func(string, string) bool{
		tool: func(string, string) bool { return true },
	}
}

func collectEvents(ch <-chan StepEvent) []StepEvent {
	var out []StepEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func hasEvent(events []StepEvent, typ StepEventType) bool {
	for _, ev := range events {
		if ev.Type == typ {
			return true
		}
	}
	return false
}

// TestApproval_Approved_ExecutesTool verifies an approved tool call is executed
// and no denial is emitted.
func TestApproval_Approved_ExecutesTool(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{toolCallEvt(0, "tc-1", "deleteFile", `{"path":"/tmp/x"}`), finishEvt(FinishReasonToolCalls)},
		{textEvt("done"), finishEvt(FinishReasonStop)},
	}}
	exec := &mockExecutor{results: map[string]string{"deleteFile": `{"ok":true}`}}
	approver := &fixedApprover{approved: true}

	events := collectEvents(Run(context.Background(), RunParams{
		Model:        model,
		Tools:        &ToolSet{Executor: exec},
		ToolApproval: requireApproval("deleteFile"),
		Approver:     approver,
		MaxSteps:     5,
	}))

	if !hasEvent(events, StepEventToolApprovalRequest) {
		t.Error("expected a tool-approval-request event")
	}
	if hasEvent(events, StepEventToolOutputDenied) {
		t.Error("did not expect a tool-output-denied event for an approved call")
	}
	if len(exec.called) != 1 || exec.called[0] != "deleteFile" {
		t.Errorf("expected deleteFile to execute once, got %v", exec.called)
	}
	if len(approver.requests) != 1 {
		t.Errorf("expected exactly one approval request, got %d", len(approver.requests))
	}
}

// TestApproval_Denied_SkipsToolAndEmitsDenial verifies a denied tool call is not
// executed and surfaces a denial with the denied output.
func TestApproval_Denied_SkipsToolAndEmitsDenial(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{toolCallEvt(0, "tc-1", "deleteFile", `{"path":"/tmp/x"}`), finishEvt(FinishReasonToolCalls)},
		{textEvt("acknowledged"), finishEvt(FinishReasonStop)},
	}}
	exec := &mockExecutor{}
	approver := &fixedApprover{approved: false, reason: "not allowed"}

	events := collectEvents(Run(context.Background(), RunParams{
		Model:        model,
		Tools:        &ToolSet{Executor: exec},
		ToolApproval: requireApproval("deleteFile"),
		Approver:     approver,
		MaxSteps:     5,
	}))

	if !hasEvent(events, StepEventToolApprovalRequest) {
		t.Error("expected a tool-approval-request event")
	}
	if !hasEvent(events, StepEventToolOutputDenied) {
		t.Error("expected a tool-output-denied event for a denied call")
	}
	if len(exec.called) != 0 {
		t.Errorf("expected the tool NOT to execute when denied, got %v", exec.called)
	}
	var denied *ToolResult
	for _, ev := range events {
		if ev.Type == StepEventToolResult && ev.ToolResult != nil && ev.ToolResult.ID == "tc-1" {
			denied = ev.ToolResult
		}
	}
	if denied == nil {
		t.Fatal("expected a tool result for the denied call")
	}
	if denied.Output != `{"error":"tool approval denied"}` {
		t.Errorf("denied output = %q, want denied JSON", denied.Output)
	}
	if !errors.Is(denied.Error, tool.ErrDenied) {
		t.Errorf("denied error = %v, want tool.ErrDenied", denied.Error)
	}
}

// TestApproval_NoResponder_Suspends verifies that without an in-process
// responder the run returns its pending approval instead of converting the
// absence of a response into a denial.
func TestApproval_NoResponder_Suspends(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{toolCallEvt(0, "tc-1", "deleteFile", `{}`), finishEvt(FinishReasonToolCalls)},
		{textEvt("ok"), finishEvt(FinishReasonStop)},
	}}
	exec := &mockExecutor{}

	events := collectEvents(Run(context.Background(), RunParams{
		Model:        model,
		Tools:        &ToolSet{Executor: exec},
		ToolApproval: requireApproval("deleteFile"),
		Approver:     nil,
		MaxSteps:     5,
	}))

	if !hasEvent(events, StepEventToolApprovalRequest) {
		t.Error("expected approval request when no approver is configured")
	}
	if hasEvent(events, StepEventToolOutputDenied) || hasEvent(events, StepEventToolResult) {
		t.Error("a suspended call must not be denied or executed")
	}
	if !hasEvent(events, StepEventStepEnd) || !hasEvent(events, StepEventDone) {
		t.Error("suspension must close the invocation cleanly with its partial step")
	}
	if len(exec.called) != 0 {
		t.Errorf("expected no execution without an approver, got %v", exec.called)
	}
}

func TestApproval_ResumesApprovedCallFromHistory(t *testing.T) {
	model := &recordingModel{mockModel: mockModel{calls: [][]StreamEvent{
		{textEvt("continued"), finishEvt(FinishReasonStop)},
	}}}
	exec := &mockExecutor{results: map[string]string{"deleteFile": `{"ok":true}`}}
	events := collectEvents(Run(context.Background(), RunParams{
		Model: model,
		Tools: &ToolSet{Executor: exec},
		Request: Request{Messages: []Message{
			{Role: "assistant", Content: []ContentPart{{
				Type: "tool_call", ToolCallID: "tc-1", ToolCallName: "deleteFile",
				ToolCallArgs: json.RawMessage(`{"path":"/tmp/x"}`),
			}}},
			{Role: "user", Content: []ContentPart{{
				Type: "tool_approval_response", ToolApprovalID: "tc-1", ToolApprovalApproved: true,
			}}},
		}},
		MaxSteps: 1,
	}))

	if len(exec.called) != 1 || exec.called[0] != "deleteFile" {
		t.Fatalf("resumed execution calls = %v", exec.called)
	}
	if !hasEvent(events, StepEventToolResult) {
		t.Fatal("expected resumed tool result event")
	}
	if len(model.requests) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.requests))
	}
	var sawResult, sawApprovalPart bool
	for _, message := range model.requests[0].Messages {
		for _, part := range message.Content {
			sawResult = sawResult || part.Type == "tool_result" && part.ToolResultID == "tc-1"
			sawApprovalPart = sawApprovalPart || part.Type == "tool_approval_response"
		}
	}
	if !sawResult || sawApprovalPart {
		t.Fatalf("provider history result=%v approvalPart=%v, want result only", sawResult, sawApprovalPart)
	}
}

func TestApproval_ResumesDeniedCallFromHistoryWithoutExecution(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{textEvt("continued"), finishEvt(FinishReasonStop)},
	}}
	exec := &mockExecutor{}
	events := collectEvents(Run(context.Background(), RunParams{
		Model: model,
		Tools: &ToolSet{Executor: exec},
		Request: Request{Messages: []Message{
			{Role: "assistant", Content: []ContentPart{{
				Type: "tool_call", ToolCallID: "tc-1", ToolCallName: "deleteFile", ToolCallArgs: json.RawMessage(`{}`),
			}}},
			{Role: "user", Content: []ContentPart{{
				Type: "tool_approval_response", ToolApprovalID: "tc-1", ToolApprovalReason: "operator denied",
			}}},
		}},
		MaxSteps: 1,
	}))

	if len(exec.called) != 0 {
		t.Fatalf("denied resumed call executed: %v", exec.called)
	}
	if !hasEvent(events, StepEventToolOutputDenied) || !hasEvent(events, StepEventToolResult) {
		t.Fatal("expected denial and denied tool-result events")
	}
}

// TestApproval_ContextCancel_NoDeadlock verifies the engine does not deadlock
// when the context is cancelled while an approval decision is pending. The
// blocking approver only returns once the context is cancelled, so the run must
// still drain to completion.
func TestApproval_ContextCancel_NoDeadlock(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{toolCallEvt(0, "tc-1", "deleteFile", `{}`), finishEvt(FinishReasonToolCalls)},
		{textEvt("after"), finishEvt(FinishReasonStop)},
	}}
	exec := &mockExecutor{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := Run(ctx, RunParams{
		Model:        model,
		Tools:        &ToolSet{Executor: exec},
		ToolApproval: requireApproval("deleteFile"),
		Approver:     blockingApprover{},
		MaxSteps:     5,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range ch {
			// Cancel once the approval request surfaces; this unblocks the
			// approver, which then reports the cancellation.
			if ev.Type == StepEventToolApprovalRequest {
				cancel()
			}
		}
	}()

	select {
	case <-done:
		// Channel drained and closed — no deadlock.
	case <-time.After(3 * time.Second):
		t.Fatal("engine deadlocked awaiting approval after context cancel")
	}
}
