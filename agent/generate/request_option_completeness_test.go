package ai

import (
	"reflect"
	"testing"
)

// TestGenerateTextRequest_OptionCompleteness asserts that every field of
// GenerateTextRequest is reachable through a functional Option or is explicitly
// exempted. Adding a field without either fails this test, keeping the option
// surface and the struct a single source of truth in lockstep.
func TestGenerateTextRequest_OptionCompleteness(t *testing.T) {
	// Fields settable via a dedicated WithX option (Settings is covered by the
	// sub-field options WithTemperature/WithMaxTokens/WithTopP).
	optioned := map[string]bool{
		"Model":                   true, // WithModel
		"Instructions":            true, // WithInstructions
		"Messages":                true, // WithMessages
		"Tools":                   true, // WithTools
		"ToolChoice":              true, // WithToolChoice
		"StopWhen":                true, // WithStopWhen
		"Output":                  true, // WithOutput
		"Settings":                true, // WithTemperature/WithMaxTokens/WithTopP
		"MaxSteps":                true, // WithMaxSteps
		"ProviderOptions":         true, // WithProviderOptions
		"PrepareStep":             true, // WithPrepareStep
		"RepairToolCall":          true, // WithRepairToolCall
		"ActiveTools":             true, // WithActiveTools
		"ToolsContext":            true, // WithToolsContext
		"RuntimeContext":          true, // WithRuntimeContext
		"ToolApproval":            true, // WithToolApproval
		"ToolApprovalKey":         true, // WithToolApprovalKey
		"ToolApprovalReplayGuard": true, // WithToolApprovalReplayGuard
		"OnStepEnd":               true, // WithOnStepEnd
		"OnEnd":                   true, // WithOnEnd
		"OnChunk":                 true, // WithOnChunk
		"OnError":                 true, // WithOnError
		"SmoothStream":            true, // WithSmoothStream
		"Middlewares":             true, // WithMiddleware
		"ParallelToolExecution":   true, // WithParallelToolExecution
		"MaxParallelTools":        true, // WithMaxParallelTools
		"Logger":                  true, // WithLogger
		"TraceContent":            true, // WithTraceContent
	}
	// Fields intentionally without a top-level option, with the reason.
	exempt := map[string]bool{
		// Lower-level pairing for ToolApproval; advanced callers set it on the
		// struct directly rather than through an option.
		"ToolApprovalResponder": true,
	}

	typ := reflect.TypeOf(GenerateTextRequest{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !optioned[name] && !exempt[name] {
			t.Errorf(
				"GenerateTextRequest.%s has no Option and no exemption — add a WithX option or add it to the exempt list with a reason",
				name,
			)
		}
	}

	// Settings is itself a struct; every one of its sub-fields must also have an
	// option so the "Settings covered" claim above is genuine, not top-level only.
	settingsOptioned := map[string]bool{
		"Temperature":   true, // WithTemperature
		"MaxTokens":     true, // WithMaxTokens
		"TopP":          true, // WithTopP
		"TopK":          true, // WithTopK
		"Seed":          true, // WithSeed
		"StopSequences": true, // WithStopSequences
	}
	st := reflect.TypeOf(CallSettings{})
	for i := 0; i < st.NumField(); i++ {
		name := st.Field(i).Name
		if !settingsOptioned[name] {
			t.Errorf("CallSettings.%s has no WithX option — add one or update the completeness guard", name)
		}
	}
}

// TestWithMaxParallelTools_NoSideEffect verifies the option sets only
// MaxParallelTools and no longer silently enables parallel execution.
func TestWithMaxParallelTools_NoSideEffect(t *testing.T) {
	var req GenerateTextRequest
	WithMaxParallelTools(4)(&req)
	if req.MaxParallelTools != 4 {
		t.Errorf("MaxParallelTools = %d, want 4", req.MaxParallelTools)
	}
	if req.ParallelToolExecution {
		t.Error("WithMaxParallelTools must not flip ParallelToolExecution — enable it explicitly")
	}
}
