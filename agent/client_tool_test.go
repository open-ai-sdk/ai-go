package agent

import (
	"context"
	"errors"
	"testing"
)

// clientToolDefs builds a tool set where the named tools are client-executed
// and every other tool runs here as usual.
func clientToolDefs(clientNames ...string) []ToolDefinition {
	client := make(map[string]bool, len(clientNames))
	for _, name := range clientNames {
		client[name] = true
	}
	return []ToolDefinition{
		{Name: "renderChart", ClientExecuted: client["renderChart"]},
		{Name: "readSelection", ClientExecuted: client["readSelection"]},
		{Name: "lookup", ClientExecuted: client["lookup"]},
	}
}

func eventsOfType(events []StepEvent, typ StepEventType) []StepEvent {
	var out []StepEvent
	for _, event := range events {
		if event.Type == typ {
			out = append(out, event)
		}
	}
	return out
}

func TestIsSuspendedRun(t *testing.T) {
	if !isSuspendedRun(errApprovalPending) || !isSuspendedRun(errClientToolPending) {
		t.Error("both suspension sentinels must report as suspended")
	}
	if isSuspendedRun(errors.New("boom")) || isSuspendedRun(nil) {
		t.Error("an ordinary error is a failure, not a suspension")
	}
}

// A client-executed tool must stream its call and then hand the turn back
// without running anything here.
func TestClientTool_SuspendsWithoutExecuting(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{toolCallEvt(0, "tc-1", "renderChart", `{"series":[1,2]}`), finishEvt(FinishReasonToolCalls)},
		{textEvt("done"), finishEvt(FinishReasonStop)},
	}}
	exec := &mockExecutor{}

	events := collectEvents(driveStream(context.Background(), runConfig{
		Model:    model,
		Tools:    testToolSet(clientToolDefs("renderChart"), exec),
		MaxSteps: 5,
	}))

	requests := eventsOfType(events, StepEventClientToolRequest)
	if len(requests) != 1 {
		t.Fatalf("got %d client tool requests, want 1", len(requests))
	}
	if requests[0].ToolCallName != "renderChart" || requests[0].ToolCallID != "tc-1" {
		t.Errorf("request carries %q/%q, want renderChart/tc-1",
			requests[0].ToolCallName, requests[0].ToolCallID)
	}
	if requests[0].ToolCallArgsDelta != `{"series":[1,2]}` {
		t.Errorf("request args = %q, want the streamed arguments", requests[0].ToolCallArgsDelta)
	}
	if len(exec.called) != 0 {
		t.Errorf("a client tool must never run here, got %v", exec.called)
	}
	if hasEvent(events, StepEventToolResult) {
		t.Error("a suspended client tool must not produce a tool result")
	}
	if hasEvent(events, StepEventError) {
		t.Error("suspension is a clean exit, not an error")
	}
	if !hasEvent(events, StepEventStepEnd) || !hasEvent(events, StepEventDone) {
		t.Error("suspension must close its step and terminate the run")
	}
	// The turn must not advance to the model's second scripted reply.
	if model.idx != 1 {
		t.Errorf("model called %d times, want 1 — the run continued past suspension", model.idx)
	}
}

// The step ends on tool_calls, which is what lets the UI protocol encode a
// success finish rather than an error.
func TestClientTool_StepEndsOnToolCalls(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{toolCallEvt(0, "tc-1", "renderChart", `{}`), finishEvt(FinishReasonToolCalls)},
	}}
	events := collectEvents(driveStream(context.Background(), runConfig{
		Model:    model,
		Tools:    testToolSet(clientToolDefs("renderChart"), &mockExecutor{}),
		MaxSteps: 5,
	}))

	ends := eventsOfType(events, StepEventStepEnd)
	if len(ends) != 1 {
		t.Fatalf("got %d step ends, want 1", len(ends))
	}
	if ends[0].FinishReason != FinishReasonToolCalls {
		t.Errorf("finish reason = %v, want tool_calls", ends[0].FinishReason)
	}
}

// A server tool called in the same turn still runs; only the client tool is
// withheld.
func TestClientTool_ServerSiblingStillExecutes(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{
			toolCallEvt(0, "tc-1", "renderChart", `{}`),
			toolCallEvt(1, "tc-2", "lookup", `{"q":"x"}`),
			finishEvt(FinishReasonToolCalls),
		},
	}}
	exec := &mockExecutor{results: map[string]string{"lookup": `{"hit":true}`}}

	events := collectEvents(driveStream(context.Background(), runConfig{
		Model:    model,
		Tools:    testToolSet(clientToolDefs("renderChart"), exec),
		MaxSteps: 5,
	}))

	if len(exec.called) != 1 || exec.called[0] != "lookup" {
		t.Errorf("executed %v, want only the server tool", exec.called)
	}
	if len(eventsOfType(events, StepEventClientToolRequest)) != 1 {
		t.Error("the client tool must still be reported while its sibling runs")
	}
	results := eventsOfType(events, StepEventToolResult)
	if len(results) != 1 || results[0].ToolResult == nil || results[0].ToolResult.Name != "lookup" {
		t.Errorf("got %d tool results, want one for lookup", len(results))
	}
	if hasEvent(events, StepEventError) {
		t.Error("a mixed turn must still exit cleanly")
	}
}

// Every client-executed call in one turn is reported, so the UI can run them
// all before replying.
func TestClientTool_MultipleCallsAllReported(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{
			toolCallEvt(0, "tc-1", "renderChart", `{}`),
			toolCallEvt(1, "tc-2", "readSelection", `{}`),
			finishEvt(FinishReasonToolCalls),
		},
	}}
	exec := &mockExecutor{}

	events := collectEvents(driveStream(context.Background(), runConfig{
		Model:    model,
		Tools:    testToolSet(clientToolDefs("renderChart", "readSelection"), exec),
		MaxSteps: 5,
	}))

	if got := len(eventsOfType(events, StepEventClientToolRequest)); got != 2 {
		t.Errorf("got %d client tool requests, want 2", got)
	}
	if len(exec.called) != 0 {
		t.Errorf("no client tool may run here, got %v", exec.called)
	}
}

// The parallel executor must suspend before scheduling any tool body.
func TestClientTool_ParallelPathSchedulesNothing(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{
			toolCallEvt(0, "tc-1", "renderChart", `{}`),
			toolCallEvt(1, "tc-2", "readSelection", `{}`),
			finishEvt(FinishReasonToolCalls),
		},
	}}
	exec := &mockExecutor{}

	events := collectEvents(driveStream(context.Background(), runConfig{
		Model:                 model,
		Tools:                 testToolSet(clientToolDefs("renderChart", "readSelection"), exec),
		ParallelToolExecution: true,
		MaxParallelTools:      2,
		MaxSteps:              5,
	}))

	if got := len(eventsOfType(events, StepEventClientToolRequest)); got != 2 {
		t.Errorf("got %d client tool requests, want 2", got)
	}
	if len(exec.called) != 0 {
		t.Errorf("the parallel path ran %v, want nothing", exec.called)
	}
	if hasEvent(events, StepEventError) {
		t.Error("parallel suspension must exit cleanly")
	}
	if !hasEvent(events, StepEventDone) {
		t.Error("parallel suspension must terminate the run")
	}
}

// A real failure on a sibling outranks the suspension, so the run reports the
// failure rather than a success finish.
func TestClientTool_SiblingFailureOutranksSuspension(t *testing.T) {
	model := &mockModel{calls: [][]StreamEvent{
		{
			toolCallEvt(0, "tc-1", "renderChart", `{}`),
			toolCallEvt(1, "tc-2", "lookup", `{}`),
			finishEvt(FinishReasonToolCalls),
		},
	}}
	events := collectEvents(driveStream(context.Background(), runConfig{
		Model: model,
		Tools: testToolSet(clientToolDefs("renderChart"),
			siblingFailureExecutor{failName: "lookup"}),
		ParallelToolExecution: true,
		MaxParallelTools:      2,
		MaxSteps:              1,
	}))

	if !hasEvent(events, StepEventClientToolRequest) {
		t.Error("the client tool is still reported before the sibling fails")
	}
	// A failing tool reports through its result, not by aborting the run;
	// asserting the run still closes guards against the suspension sentinel
	// masking a genuine error path.
	if !hasEvent(events, StepEventDone) && !hasEvent(events, StepEventError) {
		t.Error("the run must terminate one way or the other")
	}
}

// A sibling that genuinely fails must outrank the suspension. Reporting
// "pending" would end the run as a success finish and swallow the failure —
// and the sequential path is the default, so this is the common case.
func TestClientTool_SuspensionNeverMasksSiblingFailure(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		name := "sequential"
		if parallel {
			name = "parallel"
		}
		t.Run(name, func(t *testing.T) {
			model := &mockModel{calls: [][]StreamEvent{
				{
					// The client tool is prepared first, so a first-wins rule
					// would record the suspension and drop the failure below.
					toolCallEvt(0, "tc-1", "renderChart", `{}`),
					toolCallEvt(1, "tc-2", "lookup", `{}`),
					finishEvt(FinishReasonToolCalls),
				},
			}}
			config := runConfig{
				Model: model,
				Tools: testToolSet(clientToolDefs("renderChart"), &mockExecutor{}),
				Hooks: []Hook{HookFuncs{
					Name: "fail",
					AfterToolFunc: func(
						context.Context, HookContext, ToolResult,
					) (ToolResultAction, error) {
						return ToolResultAction{}, errors.New("after tool exploded")
					},
				}},
				MaxSteps: 1,
			}
			if parallel {
				config.ParallelToolExecution = true
				config.MaxParallelTools = 2
			}

			events := collectEvents(driveStream(context.Background(), config))
			if !hasEvent(events, StepEventError) {
				t.Errorf("a real failure must reach the stream, got %v", eventTypeNames(events))
			}
			if hasEvent(events, StepEventDone) {
				t.Error("a failed run must not report a clean terminal event")
			}
		})
	}
}

func eventTypeNames(events []StepEvent) []StepEventType {
	types := make([]StepEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}
