package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/tool"
)

type preparedToolCall struct {
	tc         toolCallState
	def        ToolDefinition // resolved once here; execute steps reuse it, no re-scan
	invalidErr error
}

func preparedToolCallStates(prepared []preparedToolCall) []toolCallState {
	if len(prepared) == 0 {
		return nil
	}
	states := make([]toolCallState, 0, len(prepared))
	for _, preparedCall := range prepared {
		states = append(states, preparedCall.tc)
	}
	return states
}

func prepareToolCalls(
	ctx context.Context,
	tools *ToolSet,
	repair ToolCallRepairFunc,
	req Request,
	toolCalls []toolCallState,
) []preparedToolCall {
	stepTools := toolSetForStep(tools, req.Tools)
	prepared := make([]preparedToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		fixed, def, err := validateAndRepairToolCall(ctx, stepTools, repair, req, tc)
		prepared = append(prepared, preparedToolCall{
			tc:         fixed,
			def:        def,
			invalidErr: err,
		})
	}
	return prepared
}

func validateAndRepairToolCall(
	ctx context.Context,
	tools *ToolSet,
	repair ToolCallRepairFunc,
	req Request,
	tc toolCallState,
) (toolCallState, ToolDefinition, error) {
	def, err := validateToolCall(tools, tc)
	if err == nil || repair == nil {
		return tc, def, err
	}

	repaired, repairErr := repair(ctx, ToolCallRepairContext{
		Instructions: req.Instructions,
		Messages:     snapshotMessages(req.Messages),
		ToolCall: ToolCallInfo{
			ID:               tc.id,
			Name:             tc.name,
			Args:             append(json.RawMessage(nil), tc.args...),
			ArgsSet:          true,
			ThoughtSignature: tc.thoughtSignature,
		},
		Tools: snapshotToolDefinitionsForCallback(tools),
		Error: err,
	})
	if repairErr != nil {
		return tc, def, repairErr
	}
	if repaired == nil {
		return tc, def, err
	}

	if repaired.ID != "" {
		tc.id = repaired.ID
	}
	if repaired.Name != "" {
		tc.name = repaired.Name
	}
	// Args was historically enough for the public repair callback to signal an
	// override. ArgsSet additionally permits an explicit nil/empty override.
	if repaired.ArgsSet || repaired.Args != nil {
		tc.args = string(repaired.Args)
	}
	if repaired.ThoughtSignature != "" {
		tc.thoughtSignature = repaired.ThoughtSignature
	}

	def, err = validateToolCall(tools, tc)
	return tc, def, err
}

// validateToolCall checks tc against tools and, on success, returns the
// matched ToolDefinition so callers resolve a tool call's definition exactly
// once instead of validating and then re-scanning Definitions during
// execution for ToModelOutput/Timeout. The zero ToolDefinition is returned
// when tools carries no Definitions at all (any tool name is accepted, same
// as before Lookup existed).
func validateToolCall(tools *ToolSet, tc toolCallState) (ToolDefinition, error) {
	if tools == nil {
		return ToolDefinition{}, &NoSuchToolError{
			ToolName:       tc.name,
			AvailableTools: nil,
		}
	}
	if len(tools.Definitions) == 0 {
		if err := invalidToolArgumentsError(tc.name, tc.args); err != nil {
			return ToolDefinition{}, err
		}
		return ToolDefinition{}, nil
	}
	def, ok := tools.Lookup(tc.name)
	if !ok {
		return ToolDefinition{}, &NoSuchToolError{
			ToolName:       tc.name,
			AvailableTools: toolDefinitionNames(tools),
		}
	}
	if err := invalidToolArgumentsError(tc.name, tc.args); err != nil {
		return ToolDefinition{}, err
	}
	return def, nil
}

func invalidToolArgumentsError(toolName, args string) *ToolInputError {
	if args == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(args), &decoded); err != nil {
		return &ToolInputError{
			ToolName: toolName,
			Input:    json.RawMessage(args),
			Cause:    err,
		}
	}
	return nil
}

func invalidToolCallOutput(tc toolCallState, err error) string {
	var noSuchToolErr *NoSuchToolError
	if errors.As(err, &noSuchToolErr) {
		return fmt.Sprintf(`{"error":%q}`, fmt.Sprintf("unknown tool %q", noSuchToolErr.ToolName))
	}

	var invalidArgsErr *ToolInputError
	if errors.As(err, &invalidArgsErr) {
		return fmt.Sprintf(`{"error":%q}`, fmt.Sprintf("invalid JSON arguments for tool %q", invalidArgsErr.ToolName))
	}

	return fmt.Sprintf(`{"error":%q}`, err.Error())
}

func toolDefinitionNames(tools *ToolSet) []string {
	if tools == nil || len(tools.Definitions) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools.Definitions))
	for _, def := range tools.Definitions {
		names = append(names, def.Name)
	}
	return names
}

func toolSetForStep(tools *ToolSet, activeDefs []ToolDefinition) *ToolSet {
	if tools == nil {
		return nil
	}
	if len(tools.Definitions) == 0 {
		return tools.Restrict(nil)
	}
	if len(activeDefs) == 0 {
		return nil
	}
	return tools.Restrict(activeDefs)
}

// executeToolCall invokes tools.Executor for a single validated call. def is
// the ToolDefinition resolved during validation; its Timeout, if set, bounds
// this call — the default (zero) leaves ctx as the caller's, since agent
// tools may legitimately run for minutes and an SDK-imposed default would be
// a silent behavior change.
func executeToolCall(ctx context.Context, tools *ToolSet, tc toolCallState, def ToolDefinition) *ToolResult {
	result := &ToolResult{ID: tc.id, Name: tc.name, Args: tc.args}
	if tools == nil {
		result.Error = &tool.ExecutionError{
			ToolName: tc.name,
			Cause:    errors.New("no executor"),
		}
		result.Output = fmt.Sprintf(`{"error":"no executor for tool %q"}`, tc.name)
		return result
	}
	// Inject tool call ID into context so downstream code (e.g. approval managers) can correlate.
	execCtx := tool.WithToolCallID(ctx, tc.id)
	if def.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(execCtx, def.Timeout)
		defer cancel()
	}
	output, err := tools.Invoke(execCtx, tc.name, json.RawMessage(tc.args))
	if err != nil {
		result.Error = classifyToolError(tc.name, err)
		result.Output = fmt.Sprintf(`{"error":%q}`, err.Error())
	} else {
		result.Output = string(output)
	}
	return result
}

func classifyToolError(toolName string, err error) error {
	if errors.Is(err, tool.ErrInput) ||
		errors.Is(err, tool.ErrExecution) ||
		errors.Is(err, tool.ErrDenied) ||
		errors.Is(err, tool.ErrNoSuchTool) {
		return err
	}
	return &tool.ExecutionError{ToolName: toolName, Cause: err}
}
