package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/open-ai-sdk/ai-go/internal/engine"
	"github.com/open-ai-sdk/ai-go/internal/tracing"
)

// GenerateText runs a full tool loop and returns the aggregated result.
func GenerateText(ctx context.Context, req GenerateTextRequest) (*GenerateTextResult, error) {
	if err := validateToolsContext(req); err != nil {
		return nil, err
	}
	req.SmoothStream = nil
	return StreamText(ctx, req).Consume()
}

func validateToolsContext(req GenerateTextRequest) error {
	if req.Tools == nil {
		return nil
	}
	for _, tool := range req.Tools.Definitions {
		if tool.ContextSchema == nil {
			continue
		}
		value, found := req.ToolsContext[tool.Name]
		if !found {
			return fmt.Errorf("ai: missing context for tool %q", tool.Name)
		}
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("ai: context for tool %q must be an object", tool.Name)
		}
		if required, ok := tool.ContextSchema["required"].([]any); ok {
			for _, raw := range required {
				key, ok := raw.(string)
				if !ok {
					continue
				}
				if _, ok := object[key]; !ok {
					return fmt.Errorf("ai: context for tool %q missing required field %q", tool.Name, key)
				}
			}
		}
	}
	return nil
}

// StreamText runs the tool loop and returns its live and aggregate views.
func StreamText(ctx context.Context, req GenerateTextRequest) *StreamResult {
	if err := validateToolsContext(req); err != nil {
		return NewStreamResultWithTools(erroredEventChannel(err), req.Tools)
	}
	if len(req.Middlewares) > 0 {
		req.Model = WrapLanguageModel(req.Model, req.Middlewares...)
		req.Middlewares = nil
	}
	ch := engine.Run(ctx, runParams(req))
	if req.SmoothStream != nil {
		ch = req.SmoothStream.Transform(ctx, ch)
	}
	return NewStreamResultWithTools(ch, req.Tools)
}

func erroredEventChannel(err error) <-chan StepEvent {
	ch := make(chan StepEvent, 1)
	ch <- StepEvent{Type: StepEventError, Error: err}
	close(ch)
	return ch
}

func runParams(req GenerateTextRequest) engine.RunParams {
	if req.StopWhen == nil {
		req.StopWhen = IsStepCount(1)
	}

	modelRequest := LanguageModelRequest{
		Instructions:    req.Instructions,
		Messages:        req.Messages,
		ToolChoice:      req.ToolChoice,
		Output:          req.Output,
		Settings:        req.Settings,
		ProviderOptions: req.ProviderOptions,
		ToolsContext:    req.ToolsContext,
		RuntimeContext:  req.RuntimeContext,
	}

	var tools *ToolSet
	if req.Tools != nil {
		modelRequest.Tools = req.Tools.Definitions
		if len(req.ActiveTools) > 0 {
			modelRequest.Tools = filterActiveTools(modelRequest.Tools, req.ActiveTools)
		}
		tools = &ToolSet{
			Definitions: req.Tools.Definitions,
			Executor: contextualExecutor{
				executor:       req.Tools.Executor,
				toolsContext:   req.ToolsContext,
				runtimeContext: req.RuntimeContext,
			},
		}
	}

	approval := make(map[string]func(string, string) bool, len(req.ToolApproval))
	for name, policy := range req.ToolApproval {
		approval[name] = func(tool, args string) bool {
			return policy(tool, json.RawMessage(args)) == ApprovalRequired
		}
	}
	var approver engine.ApprovalResponder
	if req.ToolApprovalResponder != nil {
		approver = approvalResponder{fn: req.ToolApprovalResponder}
	}

	return engine.RunParams{
		Model:                 req.Model,
		Request:               modelRequest,
		Tools:                 tools,
		StopWhen:              req.StopWhen,
		MaxSteps:              req.MaxSteps,
		PrepareStep:           req.PrepareStep,
		RepairToolCall:        req.RepairToolCall,
		ToolApproval:          approval,
		Approver:              approver,
		Callbacks:             lifecycleCallbacks(req),
		ParallelToolExecution: req.ParallelToolExecution,
		MaxParallelTools:      req.MaxParallelTools,
		Logger:                req.Logger,
		Tracer:                tracing.NewTracer(),
		TraceContent:          req.TraceContent,
	}
}

func filterActiveTools(tools []ToolDefinition, active []string) []ToolDefinition {
	set := make(map[string]bool, len(active))
	for _, name := range active {
		set[name] = true
	}
	filtered := make([]ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		if set[tool.Name] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

type contextualExecutor struct {
	executor       ToolExecutor
	toolsContext   ToolsContext
	runtimeContext RuntimeContext
}

func (e contextualExecutor) Execute(ctx context.Context, name, args string) (string, error) {
	return e.executor.Execute(withToolContexts(ctx, e.toolsContext[name], e.runtimeContext), name, args)
}

type approvalResponder struct{ fn ToolApprovalResponder }

func (r approvalResponder) RequestApproval(
	ctx context.Context,
	request engine.ApprovalRequest,
) (engine.ApprovalResponse, error) {
	response, err := r.fn(ctx, ToolApprovalRequest{
		ApprovalID: request.ApprovalID,
		ToolCallID: request.ToolCallID,
		ToolName:   request.ToolName,
		Args:       json.RawMessage(request.Args),
	})
	return engine.ApprovalResponse{
		ApprovalID: response.ApprovalID,
		Approved:   response.Approved,
		Reason:     response.Reason,
	}, err
}
