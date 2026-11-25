package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/open-ai-sdk/ai-go/tool"
)

type abortingExecutor struct {
	cause    error
	siblings *atomic.Int32
	calls    *atomic.Int32
}

func (e abortingExecutor) Execute(_ context.Context, name, _ string) (string, error) {
	if e.calls != nil {
		e.calls.Add(1)
	}
	if name != "uncertain" {
		e.siblings.Add(1)
		return "", errors.New("unexpected sibling execution")
	}
	return "", &tool.AbortRunError{Cause: e.cause}
}

func TestToolExecutionAbortStopsRunWithoutToolResult(t *testing.T) {
	for _, tc := range []struct {
		name     string
		parallel bool
	}{
		{"sequential", false},
		{"queued parallel", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := errors.New("effect outcome not recorded")
			var siblings atomic.Int32
			events := driveStream(t.Context(), runConfig{
				Model:                 fixedToolCallsModel{toolNames: []string{"uncertain", "sibling"}},
				Tools:                 testToolSet([]ToolDefinition{{Name: "uncertain"}, {Name: "sibling"}}, abortingExecutor{cause: cause, siblings: &siblings}),
				ParallelToolExecution: tc.parallel,
				MaxParallelTools:      1,
				MaxSteps:              2,
			})
			var runError error
			steps := 0
			for event := range events {
				switch event.Type {
				case StepEventStepStart:
					steps++
				case StepEventToolResult:
					t.Errorf("indeterminate batch emitted a result: %+v", event.ToolResult)
				case StepEventError:
					runError = event.Error
				}
			}
			var abort *tool.AbortRunError
			if steps != 1 || !errors.As(runError, &abort) || !errors.Is(runError, cause) {
				t.Fatalf("steps=%d, error=%v; want one step and original abort cause", steps, runError)
			}
			if got := siblings.Load(); got != 0 {
				t.Fatalf("launched %d siblings after abort", got)
			}
		})
	}
}

func TestToolExecutionAbortOnApprovalResumeConsumesGrant(t *testing.T) {
	cause := errors.New("awaiting reconciliation")
	var calls atomic.Int32
	const id, name = "call-1", "uncertain"
	signature := approvalSignatureForTest(t, id, name, `{}`)
	history := []Message{
		{Role: "assistant", Content: []ContentPart{{Type: "tool_call", ToolCallID: id, ToolCallName: name, ToolCallArgs: json.RawMessage(`{}`)}}},
		{Role: "user", Content: []ContentPart{{Type: "tool_approval_response", ToolApprovalID: id, ToolApprovalSignature: signature, ToolApprovalApproved: true}}},
	}
	model := &mockModel{calls: [][]StreamEvent{{textEvt("must not run"), finishEvt(FinishReasonStop)}}}
	params := runConfig{
		Model: model, Tools: testToolSet([]ToolDefinition{{Name: name}}, abortingExecutor{cause: cause, calls: &calls}),
		ToolApproval: requireApproval(name), ApprovalKey: approvalTestKey, ApprovalReplayGuard: NewMemoryApprovalReplayGuard(),
		Request: Request{Messages: history},
	}
	events := collectEvents(driveStream(t.Context(), params))
	assertApprovalError(t, events, cause)
	if calls.Load() != 1 || model.idx != 0 || hasEvent(events, StepEventToolResult) {
		t.Fatalf("calls=%d model=%d events=%#v", calls.Load(), model.idx, events)
	}
	assertApprovalError(t, collectEvents(driveStream(t.Context(), params)), ErrApprovalReplay)
	if calls.Load() != 1 {
		t.Fatal("aborted effect executed again")
	}
}
