package generate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/open-ai-sdk/ai-go/agent"
	toolpkg "github.com/open-ai-sdk/ai-go/tool"
)

// GenerateText runs a full tool loop and returns the aggregated result.
func GenerateText(ctx context.Context, req GenerateTextRequest) (*GenerateTextResult, error) {
	if err := validateToolsContext(req); err != nil {
		return nil, wrapPromptError(err, nil, initialTranscript(req))
	}
	req.SmoothStream = nil
	result, err := StreamText(ctx, req).Consume()
	if err != nil {
		return result, wrapPromptError(err, result, initialTranscript(req))
	}
	return result, nil
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
	applyDeferredMiddlewares(&req)
	ch := agent.Stream(ctx, runParams(req))
	if req.SmoothStream != nil {
		ch = req.SmoothStream.Transform(ctx, ch)
	}
	return NewStreamResultWithTools(ch, req.Tools).withInitialMessages(initialTranscript(req))
}

func initialTranscript(req GenerateTextRequest) []Message {
	initial := make([]Message, 0, len(req.Messages)+1)
	if req.Instructions != "" {
		initial = append(initial, SystemMessage(req.Instructions))
	}
	return append(initial, req.Messages...)
}

func erroredEventChannel(err error) <-chan StepEvent {
	ch := make(chan StepEvent, 1)
	ch <- StepEvent{Type: StepEventError, Error: err}
	close(ch)
	return ch
}

func runParams(req GenerateTextRequest) agent.RunParams {
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
		if req.ActiveTools != nil {
			modelRequest.Tools = filterActiveTools(modelRequest.Tools, req.ActiveTools)
		}
		tools = &ToolSet{
			Definitions: req.Tools.Definitions,
			Executor: contextualExecutor{
				executor:       req.Tools,
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
	var approver agent.ApprovalResponder
	if req.ToolApprovalResponder != nil {
		approver = approvalResponder{fn: req.ToolApprovalResponder}
	}

	return agent.RunParams{
		Model:                 req.Model,
		Request:               modelRequest,
		Tools:                 tools,
		StopWhen:              req.StopWhen,
		MaxSteps:              req.MaxSteps,
		PrepareStep:           req.PrepareStep,
		RepairToolCall:        req.RepairToolCall,
		ToolApproval:          approval,
		ApprovalKey:           append([]byte(nil), req.ToolApprovalKey...),
		ApprovalReplayGuard:   req.ToolApprovalReplayGuard,
		Approver:              approver,
		Callbacks:             lifecycleCallbacks(req),
		ParallelToolExecution: req.ParallelToolExecution,
		MaxParallelTools:      req.MaxParallelTools,
		Logger:                req.Logger,
		Tracer:                req.Tracer,
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
	ctx = toolpkg.WithToolContext(ctx, e.toolsContext[name])
	ctx = toolpkg.WithRuntimeContext(ctx, e.runtimeContext)
	return e.executor.Execute(ctx, name, args)
}

type approvalResponder struct{ fn ToolApprovalResponder }

func (r approvalResponder) RequestApproval(
	ctx context.Context,
	request agent.ApprovalRequest,
) (agent.ApprovalResponse, error) {
	response, err := r.fn(ctx, ToolApprovalRequest{
		ApprovalID: request.ApprovalID,
		ToolCallID: request.ToolCallID,
		ToolName:   request.ToolName,
		Args:       json.RawMessage(request.Args),
	})
	return agent.ApprovalResponse{
		ApprovalID: response.ApprovalID,
		Approved:   response.Approved,
		Reason:     response.Reason,
	}, err
}
