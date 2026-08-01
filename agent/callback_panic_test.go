package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

type panicApprovalResponder struct{}

type startErrorModel struct{ err error }

func (m startErrorModel) ModelID() string { return "start-error" }

func (m startErrorModel) Stream(context.Context, Request) (<-chan StreamEvent, error) {
	return nil, m.err
}

func (panicApprovalResponder) RequestApproval(
	context.Context,
	ApprovalRequest,
) (ApprovalResponse, error) {
	panic("approval responder panic")
}

func collectRunEvents(params RunParams) []StepEvent {
	events := make([]StepEvent, 0, 8)
	for event := range Run(context.Background(), params) {
		events = append(events, event)
	}
	return events
}

func requirePanicEvent(t *testing.T, events []StepEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != StepEventError {
			continue
		}
		var panicErr *safego.PanicError
		if errors.As(event.Error, &panicErr) {
			return
		}
	}
	t.Fatalf("events contain no *safego.PanicError: %#v", events)
}

func TestObserverCallbackPanicsAreSwallowed(t *testing.T) {
	providerErr := errors.New("provider failure")
	tests := []struct {
		name   string
		params RunParams
	}{
		{
			name: "on_chunk",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{{textEvt("ok"), finishEvt(FinishReasonStop)}}},
				Callbacks: &LifecycleCallbacks{OnChunk: func(StepEvent) {
					panic("on chunk")
				}},
			},
		},
		{
			name: "on_step_end",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{{textEvt("ok"), finishEvt(FinishReasonStop)}}},
				Callbacks: &LifecycleCallbacks{OnStepEnd: func(StepEndEvent) {
					panic("on step end")
				}},
			},
		},
		{
			name: "on_end",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{{textEvt("ok"), finishEvt(FinishReasonStop)}}},
				Callbacks: &LifecycleCallbacks{OnEnd: func(EndEvent) {
					panic("on end")
				}},
			},
		},
		{
			name: "on_error",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{{{Type: StreamEventError, Error: providerErr}}}},
				Callbacks: &LifecycleCallbacks{OnError: func(error) {
					panic("on error")
				}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := collectRunEvents(test.params)
			for _, event := range events {
				var panicErr *safego.PanicError
				if event.Type == StepEventError && errors.As(event.Error, &panicErr) {
					t.Fatalf("observer panic escaped into stream: %v", event.Error)
				}
			}
			if test.name != "on_error" {
				var sawDone bool
				for _, event := range events {
					sawDone = sawDone || event.Type == StepEventDone
				}
				if !sawDone {
					t.Fatal("normal run did not finish after observer panic")
				}
			}
		})
	}
}

func TestControlCallbackPanicsFailRun(t *testing.T) {
	toolCall := []StreamEvent{
		toolCallEvt(0, "tc1", "search", `{}`),
		finishEvt(FinishReasonToolCalls),
	}
	tests := []struct {
		name   string
		params RunParams
	}{
		{
			name: "prepare_step",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{{textEvt("unused")}}},
				PrepareStep: func(PrepareStepContext) *PrepareStepResult {
					panic("prepare step")
				},
			},
		},
		{
			name: "stop_when",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{toolCall}},
				Tools: &ToolSet{Definitions: []ToolDefinition{{Name: "search"}}, Executor: &mockExecutor{}},
				StopWhen: func(int, *StepResult) bool {
					panic("stop condition")
				},
			},
		},
		{
			name: "repair_tool_call",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{{
					toolCallEvt(0, "tc1", "unknown", `{}`),
					finishEvt(FinishReasonToolCalls),
				}}},
				Tools: &ToolSet{Definitions: []ToolDefinition{{Name: "search"}}, Executor: &mockExecutor{}},
				RepairToolCall: func(context.Context, ToolCallRepairContext) (*ToolCallInfo, error) {
					panic("repair")
				},
			},
		},
		{
			name: "approval_policy",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{toolCall}},
				Tools: &ToolSet{Definitions: []ToolDefinition{{Name: "search"}}, Executor: &mockExecutor{}},
				ToolApproval: map[string]func(string, string) bool{
					"search": func(string, string) bool { panic("approval policy") },
				},
			},
		},
		{
			name: "approval_responder",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{toolCall}},
				Tools: &ToolSet{Definitions: []ToolDefinition{{Name: "search"}}, Executor: &mockExecutor{}},
				ToolApproval: map[string]func(string, string) bool{
					"search": func(string, string) bool { return true },
				},
				Approver:    panicApprovalResponder{},
				ApprovalKey: approvalTestKey,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirePanicEvent(t, collectRunEvents(test.params))
		})
	}
}

func TestToModelOutputPanicPreservesToolResultBeforeFailingRun(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(map[bool]string{false: "sequential", true: "parallel"}[parallel], func(t *testing.T) {
			events := collectRunEvents(RunParams{
				Model: &mockModel{calls: [][]StreamEvent{{
					toolCallEvt(0, "tc1", "search", `{}`),
					finishEvt(FinishReasonToolCalls),
				}}},
				Tools: &ToolSet{
					Definitions: []ToolDefinition{{
						Name: "search",
						ToModelOutput: func(string) string {
							panic("history transform")
						},
					}},
					Executor: &mockExecutor{results: map[string]string{"search": `{"ok":true}`}},
				},
				ParallelToolExecution: parallel,
			})

			resultIndex, errorIndex := -1, -1
			for index, event := range events {
				if event.Type == StepEventToolResult && event.ToolResult != nil &&
					event.ToolResult.Output == `{"ok":true}` {
					resultIndex = index
				}
				if event.Type == StepEventError {
					errorIndex = index
				}
			}
			if resultIndex < 0 || errorIndex < 0 || resultIndex >= errorIndex {
				t.Fatalf("tool result must precede control error: %#v", events)
			}
			requirePanicEvent(t, events)
		})
	}
}

func TestOnErrorObservesEveryTerminalFailureBoundary(t *testing.T) {
	sentinel := errors.New("terminal failure")
	tests := []struct {
		name   string
		params RunParams
	}{
		{
			name: "tool_set_validation",
			params: RunParams{
				Tools: &ToolSet{Definitions: []ToolDefinition{{Name: "dup"}, {Name: "dup"}}},
			},
		},
		{
			name:   "stream_start",
			params: RunParams{Model: startErrorModel{err: sentinel}},
		},
		{
			name: "control_panic",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{{textEvt("unused")}}},
				PrepareStep: func(PrepareStepContext) *PrepareStepResult {
					panic("control")
				},
			},
		},
		{
			name: "structured_output",
			params: RunParams{
				Model: &mockModel{calls: [][]StreamEvent{
					{textEvt("draft"), finishEvt(FinishReasonStop)},
					{{Type: StreamEventError, Error: sentinel}},
				}},
				Request: Request{Output: &OutputSchema{Type: "object"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var observed []error
			test.params.Callbacks = &LifecycleCallbacks{OnError: func(err error) {
				observed = append(observed, err)
			}}
			events := collectRunEvents(test.params)
			if len(observed) != 1 {
				t.Fatalf("OnError calls = %d, want 1; events=%#v", len(observed), events)
			}
		})
	}
}
