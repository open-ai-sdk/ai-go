package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type preparedToolCall struct {
	tc         toolCallState
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
		fixed, err := validateAndRepairToolCall(ctx, stepTools, repair, req, tc)
		prepared = append(prepared, preparedToolCall{
			tc:         fixed,
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
) (toolCallState, error) {
	err := validateToolCall(tools, tc)
	if err == nil || repair == nil {
		return tc, err
	}

	repaired, repairErr := repair(ctx, ToolCallRepairContext{
		Instructions: req.Instructions,
		Messages:     req.Messages,
		ToolCall: ToolCallInfo{
			ID:               tc.id,
			Name:             tc.name,
			Args:             tc.args,
			ArgsSet:          true,
			ThoughtSignature: tc.thoughtSignature,
		},
		Tools: tools,
		Error: err,
	})
	if repairErr != nil {
		return tc, repairErr
	}
	if repaired == nil {
		return tc, err
	}

	if repaired.ID != "" {
		tc.id = repaired.ID
	}
	if repaired.Name != "" {
		tc.name = repaired.Name
	}
	if repaired.ArgsSet {
		tc.args = repaired.Args
	}
	if repaired.ThoughtSignature != "" {
		tc.thoughtSignature = repaired.ThoughtSignature
	}

	return tc, validateToolCall(tools, tc)
}

func validateToolCall(tools *ToolSet, tc toolCallState) error {
	if tools == nil {
		return &NoSuchToolError{
			ToolName:       tc.name,
			AvailableTools: nil,
		}
	}
	if len(tools.Definitions) == 0 {
		if err := invalidToolArgumentsError(tc.name, tc.args); err != nil {
			return err
		}
		return nil
	}
	if _, ok := findToolDefinition(tools, tc.name); !ok {
		return &NoSuchToolError{
			ToolName:       tc.name,
			AvailableTools: toolDefinitionNames(tools),
		}
	}
	if err := invalidToolArgumentsError(tc.name, tc.args); err != nil {
		return err
	}
	return nil
}

func invalidToolArgumentsError(toolName, args string) *InvalidToolArgumentsError {
	if args == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(args), &decoded); err != nil {
		return &InvalidToolArgumentsError{
			ToolName: toolName,
			Args:     args,
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

	var invalidArgsErr *InvalidToolArgumentsError
	if errors.As(err, &invalidArgsErr) {
		return fmt.Sprintf(`{"error":%q}`, fmt.Sprintf("invalid JSON arguments for tool %q", invalidArgsErr.ToolName))
	}

	return fmt.Sprintf(`{"error":%q}`, err.Error())
}

func findToolDefinition(tools *ToolSet, name string) (*ToolDefinition, bool) {
	if tools == nil {
		return nil, false
	}
	for i := range tools.Definitions {
		if tools.Definitions[i].Name == name {
			return &tools.Definitions[i], true
		}
	}
	return nil, false
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
		return &ToolSet{Executor: tools.Executor}
	}
	if len(activeDefs) == 0 {
		return nil
	}
	return &ToolSet{
		Definitions: activeDefs,
		Executor:    tools.Executor,
	}
}

func executeToolCall(ctx context.Context, tools *ToolSet, tc toolCallState) *ToolResult {
	result := &ToolResult{ID: tc.id, Name: tc.name, Args: tc.args}
	if tools == nil || tools.Executor == nil {
		result.Output = fmt.Sprintf(`{"error":"no executor for tool %q"}`, tc.name)
		return result
	}
	// Inject tool call ID into context so downstream code (e.g. approval managers) can correlate.
	execCtx := context.WithValue(ctx, toolCallIDCtxKey, tc.id)
	output, err := tools.Executor.Execute(execCtx, tc.name, tc.args)
	if err != nil {
		result.Output = fmt.Sprintf(`{"error":%q}`, err.Error())
	} else {
		result.Output = output
	}
	return result
}
