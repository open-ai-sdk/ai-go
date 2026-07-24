package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/internal/engine"
	"github.com/open-ai-sdk/ai-go/internal/safego"
)

// GenerateText runs a full tool loop and returns the aggregated result.
func GenerateText(ctx context.Context, req GenerateTextRequest) (*GenerateTextResult, error) {
	if err := validateToolsContext(req); err != nil {
		return nil, err
	}
	ch := engine.Run(ctx, toEngineParams(req))

	result := &GenerateTextResult{}
	var currentStep *StepOutput

	for ev := range ch {
		switch ev.Type {
		case engine.StepEventStepStart:
			currentStep = &StepOutput{}

		case engine.StepEventTextDelta:
			result.Text += ev.TextDelta
			if currentStep != nil {
				currentStep.Text += ev.TextDelta
			}

		case engine.StepEventReasoningDelta:
			result.Reasoning += ev.ReasoningDelta
			if currentStep != nil {
				currentStep.Reasoning += ev.ReasoningDelta
			}

		case engine.StepEventToolCallStart:
			handleToolCallStart(ev, currentStep)

		case engine.StepEventToolCallDelta:
			handleToolCallDelta(ev, currentStep)

		case engine.StepEventToolCallReady:
			handleToolCallReady(ev, currentStep)

		case engine.StepEventToolResult:
			currentStep = handleToolResult(ev, result, currentStep)

		case engine.StepEventUsage:
			handleUsage(ev, result, currentStep)

		case engine.StepEventSource:
			handleSource(ev, result, currentStep)

		case engine.StepEventFileDelta:
			handleFileDelta(ev, result, currentStep)

		case engine.StepEventStepEnd:
			currentStep = handleStepEnd(ev, result, currentStep, req.Tools)

		case engine.StepEventStructuredOutput:
			result.StructuredOutput = ev.StructuredOutput

		case engine.StepEventError:
			return result, ev.Error
		}
	}

	result.Response = Response{Messages: ResponseMessagesForSteps(result.Steps, req.Tools)}
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

func handleToolCallStart(ev engine.StepEvent, step *StepOutput) {
	if step == nil {
		return
	}
	step.ToolCalls = append(step.ToolCalls, ToolCallOutput{
		ID:               ev.ToolCallID,
		Name:             ev.ToolCallName,
		Args:             json.RawMessage(ev.ToolCallArgsDelta),
		ThoughtSignature: ev.ThoughtSignature,
	})
}

func handleToolCallDelta(ev engine.StepEvent, step *StepOutput) {
	if step == nil || ev.ToolCallArgsDelta == "" {
		return
	}
	for i := range step.ToolCalls {
		if step.ToolCalls[i].ID == ev.ToolCallID {
			step.ToolCalls[i].Args = append(step.ToolCalls[i].Args, ev.ToolCallArgsDelta...)
			return
		}
	}
}

func handleToolCallReady(ev engine.StepEvent, step *StepOutput) {
	if step == nil {
		return
	}
	for i := range step.ToolCalls {
		if step.ToolCalls[i].ID == ev.ToolCallID {
			step.ToolCalls[i].Name = ev.ToolCallName
			if ev.ToolCallArgsDelta != "" {
				step.ToolCalls[i].Args = json.RawMessage(ev.ToolCallArgsDelta)
			}
			if ev.ThoughtSignature != "" {
				step.ToolCalls[i].ThoughtSignature = ev.ThoughtSignature
			}
			return
		}
	}
	step.ToolCalls = append(step.ToolCalls, ToolCallOutput{
		ID:               ev.ToolCallID,
		Name:             ev.ToolCallName,
		Args:             json.RawMessage(ev.ToolCallArgsDelta),
		ThoughtSignature: ev.ThoughtSignature,
	})
}

func handleToolResult(ev engine.StepEvent, result *GenerateTextResult, step *StepOutput) *StepOutput {
	if ev.ToolResult == nil {
		return step
	}
	tr := ToolResult{
		ID:      ev.ToolResult.ID,
		Name:    ev.ToolResult.Name,
		Args:    ev.ToolResult.Args,
		Output:  ev.ToolResult.Output,
		Content: fromEngineToolResultContent(ev.ToolResult.Content),
	}
	result.ToolResults = append(result.ToolResults, tr)
	if step != nil {
		step.ToolResults = append(step.ToolResults, tr)
	}
	return step
}

func handleUsage(ev engine.StepEvent, result *GenerateTextResult, step *StepOutput) {
	if ev.Usage == nil {
		return
	}
	result.Usage.InputTokens += ev.Usage.InputTokens
	result.Usage.InputTokenDetails.NoCacheTokens += ev.Usage.InputTokenDetails.NoCacheTokens
	result.Usage.OutputTokens += ev.Usage.OutputTokens
	result.Usage.OutputTokenDetails.TextTokens += ev.Usage.OutputTokenDetails.TextTokens
	result.Usage.TotalTokens += ev.Usage.TotalTokens
	result.Usage.OutputTokenDetails.ReasoningTokens += ev.Usage.OutputTokenDetails.ReasoningTokens
	result.Usage.InputTokenDetails.CacheReadTokens += ev.Usage.InputTokenDetails.CacheReadTokens
	result.Usage.InputTokenDetails.CacheWriteTokens += ev.Usage.InputTokenDetails.CacheWriteTokens
	if ev.Usage.Raw != nil {
		result.Usage.Raw = ev.Usage.Raw
	}
	if step != nil {
		step.Usage = Usage{
			InputTokens:  ev.Usage.InputTokens,
			OutputTokens: ev.Usage.OutputTokens,
			TotalTokens:  ev.Usage.TotalTokens,
			InputTokenDetails: InputTokenDetails{
				NoCacheTokens:    ev.Usage.InputTokenDetails.NoCacheTokens,
				CacheReadTokens:  ev.Usage.InputTokenDetails.CacheReadTokens,
				CacheWriteTokens: ev.Usage.InputTokenDetails.CacheWriteTokens,
			},
			OutputTokenDetails: OutputTokenDetails{
				TextTokens:      ev.Usage.OutputTokenDetails.TextTokens,
				ReasoningTokens: ev.Usage.OutputTokenDetails.ReasoningTokens,
			},
		}
	}
}

func handleSource(ev engine.StepEvent, result *GenerateTextResult, step *StepOutput) {
	if ev.Source == nil {
		return
	}
	src := Source{
		SourceType:       ev.Source.SourceType,
		ID:               ev.Source.ID,
		URL:              ev.Source.URL,
		Title:            ev.Source.Title,
		ProviderMetadata: ev.Source.ProviderMetadata,
	}
	result.Sources = append(result.Sources, src)
	if step != nil {
		step.Sources = append(step.Sources, src)
	}
}

func handleFileDelta(ev engine.StepEvent, result *GenerateTextResult, step *StepOutput) {
	if len(ev.FileData) == 0 {
		return
	}
	f := GeneratedFile{
		Data:      ev.FileData,
		MediaType: ev.FileMediaType,
	}
	result.Files = append(result.Files, f)
	if step != nil {
		step.Files = append(step.Files, f)
	}
}

func handleStepEnd(ev engine.StepEvent, result *GenerateTextResult, step *StepOutput, tools *ToolSet) *StepOutput {
	if step == nil {
		return nil
	}
	step.FinishReason = FinishReason(ev.FinishReason)
	step.RawFinishReason = ev.RawFinishReason
	step.ProviderMetadata = ev.ProviderMetadata
	step.Warnings = fromEngineWarnings(ev.Warnings)
	step.Response = Response{Messages: ResponseMessagesForStep(*step, tools)}
	result.Steps = append(result.Steps, *step)
	result.FinalStep = *step
	result.Text = step.Text
	result.Reasoning = step.Reasoning
	result.FinishReason = FinishReason(ev.FinishReason)
	result.RawFinishReason = ev.RawFinishReason
	result.ProviderMetadata = ev.ProviderMetadata
	result.Warnings = append(result.Warnings, step.Warnings...)
	return nil
}

// StreamText runs the tool loop and returns a *StreamResult for callers that
// need live streaming (e.g. SSE adapters). Use StreamResult.TextStream() for
// text deltas, StreamResult.Stream() for raw engine events, or
// StreamResult.Consume() to block and get the full aggregated result.
func StreamText(ctx context.Context, req GenerateTextRequest) *StreamResult {
	if err := validateToolsContext(req); err != nil {
		return NewStreamResultWithTools(erroredEventChannel(err), req.Tools)
	}
	ch := engine.Run(ctx, toEngineParams(req))
	if req.SmoothStream != nil {
		ch = req.SmoothStream.Transform(ctx, ch)
	}
	return NewStreamResultWithTools(ch, req.Tools)
}

// erroredEventChannel returns a closed channel yielding a single error event, so
// a pre-flight validation failure surfaces through the normal stream API
// (Consume returns the error, Stream emits it) instead of being silently dropped.
func erroredEventChannel(err error) <-chan engine.StepEvent {
	ch := make(chan engine.StepEvent, 1)
	ch <- engine.StepEvent{Type: engine.StepEventError, Error: err}
	close(ch)
	return ch
}

// toEngineParams converts a public GenerateTextRequest to engine.RunParams.
// It also wraps the ai.LanguageModel to satisfy engine.Model.
func toEngineParams(req GenerateTextRequest) engine.RunParams {
	engReq, engTools := toEngineRequest(req)
	stopWhen := toEngineStopWhen(req.StopWhen)
	engPrepareStep := toEnginePrepareStep(req.PrepareStep)
	repairToolCall := toEngineRepairToolCall(req.RepairToolCall)
	engCallbacks := toEngineLifecycleCallbacks(req)
	approval := make(map[string]func(string, string) bool, len(req.ToolApproval))
	for name, policy := range req.ToolApproval {
		approval[name] = func(tool, args string) bool { return policy(tool, json.RawMessage(args)) == ApprovalRequired }
	}
	var approver engine.ApprovalResponder
	if req.ToolApprovalResponder != nil {
		approver = approvalResponder{fn: req.ToolApprovalResponder}
	}

	return engine.RunParams{
		Model:                 &engineModelAdapter{req.Model},
		Request:               engReq,
		Tools:                 engTools,
		StopWhen:              stopWhen,
		MaxSteps:              req.MaxSteps,
		PrepareStep:           engPrepareStep,
		RepairToolCall:        repairToolCall,
		ToolApproval:          approval,
		Approver:              approver,
		Callbacks:             engCallbacks,
		ParallelToolExecution: req.ParallelToolExecution,
		MaxParallelTools:      req.MaxParallelTools,
	}
}

func toEngineRequest(req GenerateTextRequest) (engine.Request, *engine.ToolSet) {
	engReq := engine.Request{
		Instructions:    req.Instructions,
		Messages:        toEngineMessages(req.Messages),
		ProviderOptions: req.ProviderOptions,
		ToolsContext:    req.ToolsContext,
		RuntimeContext:  req.RuntimeContext,
		Settings: engine.CallSettings{
			Temperature:   req.Settings.Temperature,
			MaxTokens:     req.Settings.MaxTokens,
			TopP:          req.Settings.TopP,
			TopK:          req.Settings.TopK,
			Seed:          req.Settings.Seed,
			StopSequences: req.Settings.StopSequences,
		},
	}
	if req.Output != nil {
		engReq.Output = &engine.OutputSchema{Type: req.Output.Type, Schema: req.Output.Schema}
	}
	if req.ToolChoice != nil {
		engReq.ToolChoice = &engine.ToolChoice{Type: req.ToolChoice.Type, ToolName: req.ToolChoice.ToolName}
	}

	if req.Tools == nil {
		return engReq, nil
	}

	defs := make([]engine.ToolDefinition, len(req.Tools.Definitions))
	for i, d := range req.Tools.Definitions {
		defs[i] = engine.ToolDefinition{
			Name:          d.Name,
			Description:   d.Description,
			InputSchema:   d.InputSchema,
			ContextSchema: d.ContextSchema,
			ToModelOutput: d.ToModelOutput,
		}
	}
	engReq.Tools = defs
	if len(req.ActiveTools) > 0 {
		engReq.Tools = engineFilterTools(defs, req.ActiveTools)
	}

	return engReq, &engine.ToolSet{
		Definitions: defs,
		Executor: contextualExecutor{
			executor:       req.Tools.Executor,
			toolsContext:   req.ToolsContext,
			runtimeContext: req.RuntimeContext,
		},
	}
}

type contextualExecutor struct {
	executor       ToolExecutor
	toolsContext   ToolsContext
	runtimeContext RuntimeContext
}

type approvalResponder struct{ fn ToolApprovalResponder }

func (r approvalResponder) RequestApproval(
	ctx context.Context,
	request engine.ApprovalRequest,
) (engine.ApprovalResponse, error) {
	response, err := r.fn(
		ctx,
		ToolApprovalRequest{
			ApprovalID: request.ApprovalID,
			ToolCallID: request.ToolCallID,
			ToolName:   request.ToolName,
			Args:       json.RawMessage(request.Args),
		},
	)
	return engine.ApprovalResponse{
		ApprovalID: response.ApprovalID,
		Approved:   response.Approved,
		Reason:     response.Reason,
	}, err
}

func (e contextualExecutor) Execute(ctx context.Context, name, args string) (string, error) {
	return e.executor.Execute(withToolContexts(ctx, e.toolsContext[name], e.runtimeContext), name, args)
}

func toEngineStopWhen(stopWhen StopCondition) engine.StopCondition {
	if stopWhen == nil {
		return nil
	}
	return func(step int, r *engine.StepResult) bool {
		return stopWhen(step, &StepResult{
			HasToolCalls: r.HasToolCalls,
			ToolNames:    r.ToolNames,
			Text:         r.Text,
		})
	}
}

func toEnginePrepareStep(prepare PrepareStepFunc) engine.PrepareStepFunc {
	if prepare == nil {
		return nil
	}
	return func(ectx engine.PrepareStepContext) *engine.PrepareStepResult {
		aiCtx := PrepareStepContext{
			StepNumber:     ectx.StepNumber,
			ToolsContext:   ectx.ToolsContext,
			RuntimeContext: ectx.RuntimeContext,
		}
		for _, s := range ectx.Steps {
			aiCtx.Steps = append(aiCtx.Steps, PrepareStepInfo{
				StepNumber:   s.StepNumber,
				HasToolCalls: s.HasToolCalls,
				ToolNames:    s.ToolNames,
				Text:         s.Text,
				FinishReason: FinishReason(s.FinishReason),
			})
		}
		result := prepare(aiCtx)
		if result == nil {
			return nil
		}
		engResult := &engine.PrepareStepResult{
			ActiveTools:     result.ActiveTools,
			Instructions:    result.Instructions,
			ProviderOptions: result.ProviderOptions,
		}
		if result.Model != nil {
			engResult.Model = &engineModelAdapter{result.Model}
		}
		if result.ToolChoice != nil {
			engResult.ToolChoice = &engine.ToolChoice{
				Type:     result.ToolChoice.Type,
				ToolName: result.ToolChoice.ToolName,
			}
		}
		return engResult
	}
}

func toEngineRepairToolCall(fn RepairToolCallFunc) engine.ToolCallRepairFunc {
	if fn == nil {
		return nil
	}
	return func(ctx context.Context, input engine.ToolCallRepairContext) (*engine.ToolCallInfo, error) {
		publicInput := RepairToolCallInput{
			Instructions: input.Instructions,
			Messages:     fromEngineMessages(input.Messages),
			ToolCall: ToolCallOutput{
				ID:               input.ToolCall.ID,
				Name:             input.ToolCall.Name,
				Args:             json.RawMessage(input.ToolCall.Args),
				ThoughtSignature: input.ToolCall.ThoughtSignature,
			},
			Tools: fromEngineToolSet(input.Tools),
			Error: fromEngineToolCallError(input.Error),
		}
		repaired, err := fn(ctx, publicInput)
		if err != nil {
			return nil, err
		}
		if repaired == nil {
			return nil, nil
		}

		result := &engine.ToolCallInfo{
			ID:               input.ToolCall.ID,
			Name:             input.ToolCall.Name,
			Args:             input.ToolCall.Args,
			ThoughtSignature: input.ToolCall.ThoughtSignature,
		}
		if repaired.ID != "" {
			result.ID = repaired.ID
		}
		if repaired.Name != "" {
			result.Name = repaired.Name
		}
		if repaired.Args != nil {
			result.Args = string(repaired.Args)
			result.ArgsSet = true
		}
		if repaired.ThoughtSignature != "" {
			result.ThoughtSignature = repaired.ThoughtSignature
		}
		return result, nil
	}
}

func toEngineLifecycleCallbacks(req GenerateTextRequest) *engine.LifecycleCallbacks {
	if req.OnStepEnd == nil && req.OnEnd == nil && req.OnChunk == nil && req.OnError == nil {
		return nil
	}

	callbacks := &engine.LifecycleCallbacks{}
	if req.OnStepEnd != nil {
		callbacks.OnStepEnd = func(ev engine.StepEndEvent) {
			stepEvent := StepEndEvent{
				StepNumber:       ev.StepNumber,
				Text:             ev.Text,
				Reasoning:        ev.Reasoning,
				ToolCalls:        fromEngineToolCalls(ev.ToolCalls),
				ToolResults:      fromEngineToolResults(ev.ToolResults),
				FinishReason:     FinishReason(ev.FinishReason),
				Usage:            fromEngineUsagePtr(ev.Usage),
				ProviderMetadata: ev.ProviderMetadata,
				Warnings:         fromEngineWarnings(ev.Warnings),
			}
			stepEvent.Response = Response{Messages: ResponseMessagesForStep(StepOutput{
				Text:        stepEvent.Text,
				Reasoning:   stepEvent.Reasoning,
				ToolCalls:   stepEvent.ToolCalls,
				ToolResults: stepEvent.ToolResults,
			}, req.Tools)}
			req.OnStepEnd(stepEvent)
		}
	}
	if req.OnEnd != nil {
		callbacks.OnEnd = func(ev engine.EndEvent) {
			steps := fromEngineStepInfos(ev.Steps, req.Tools)
			endEvent := EndEvent{
				Text:             ev.Text,
				Reasoning:        ev.Reasoning,
				Steps:            steps,
				Usage:            fromEngineUsage(ev.Usage),
				FinishReason:     FinishReason(ev.FinishReason),
				ProviderMetadata: ev.ProviderMetadata,
			}
			endEvent.Response = Response{Messages: ResponseMessagesForSteps(steps, req.Tools)}
			req.OnEnd(endEvent)
		}
	}
	if req.OnChunk != nil {
		callbacks.OnChunk = func(ev engine.StepEvent) {
			req.OnChunk(toChunkEvent(ev))
		}
	}
	if req.OnError != nil {
		callbacks.OnError = req.OnError
	}
	return callbacks
}

func engineFilterTools(tools []engine.ToolDefinition, active []string) []engine.ToolDefinition {
	set := make(map[string]bool, len(active))
	for _, name := range active {
		set[name] = true
	}
	var filtered []engine.ToolDefinition
	for _, t := range tools {
		if set[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func fromEngineToolCalls(tcs []engine.ToolCallInfo) []ToolCallOutput {
	if len(tcs) == 0 {
		return nil
	}
	out := make([]ToolCallOutput, len(tcs))
	for i, tc := range tcs {
		out[i] = ToolCallOutput{
			ID:               tc.ID,
			Name:             tc.Name,
			Args:             json.RawMessage(tc.Args),
			ThoughtSignature: tc.ThoughtSignature,
		}
	}
	return out
}

func fromEngineToolSet(ts *engine.ToolSet) *ToolSet {
	if ts == nil {
		return nil
	}
	defs := make([]ToolDefinition, len(ts.Definitions))
	for i, def := range ts.Definitions {
		defs[i] = ToolDefinition{
			Name:          def.Name,
			Description:   def.Description,
			InputSchema:   def.InputSchema,
			ToModelOutput: def.ToModelOutput,
		}
	}
	return &ToolSet{
		Definitions: defs,
		Executor:    ts.Executor,
	}
}

func fromEngineToolResults(trs []engine.ToolResult) []ToolResult {
	if len(trs) == 0 {
		return nil
	}
	out := make([]ToolResult, len(trs))
	for i, tr := range trs {
		out[i] = ToolResult{
			ID: tr.ID, Name: tr.Name, Args: tr.Args,
			Output:  tr.Output,
			Content: fromEngineToolResultContent(tr.Content),
		}
	}
	return out
}

func fromEngineToolResultContent(cs []engine.ToolResultContent) []ToolResultContent {
	if len(cs) == 0 {
		return nil
	}
	out := make([]ToolResultContent, len(cs))
	for i, c := range cs {
		out[i] = ToolResultContent{Type: c.Type, Text: c.Text, Data: c.Data, MediaType: c.MediaType}
	}
	return out
}

func fromEngineUsagePtr(u *engine.Usage) *Usage {
	if u == nil {
		return nil
	}
	return &Usage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.TotalTokens,
		InputTokenDetails: InputTokenDetails{
			NoCacheTokens:    u.InputTokenDetails.NoCacheTokens,
			CacheReadTokens:  u.InputTokenDetails.CacheReadTokens,
			CacheWriteTokens: u.InputTokenDetails.CacheWriteTokens,
		},
		OutputTokenDetails: OutputTokenDetails{
			TextTokens:      u.OutputTokenDetails.TextTokens,
			ReasoningTokens: u.OutputTokenDetails.ReasoningTokens,
		},
	}
}

func fromEngineUsage(u engine.Usage) Usage {
	return Usage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.TotalTokens,
		InputTokenDetails: InputTokenDetails{
			NoCacheTokens:    u.InputTokenDetails.NoCacheTokens,
			CacheReadTokens:  u.InputTokenDetails.CacheReadTokens,
			CacheWriteTokens: u.InputTokenDetails.CacheWriteTokens,
		},
		OutputTokenDetails: OutputTokenDetails{
			TextTokens:      u.OutputTokenDetails.TextTokens,
			ReasoningTokens: u.OutputTokenDetails.ReasoningTokens,
		},
	}
}

func fromEngineStepInfos(steps []engine.StepResultInfo, tools *ToolSet) []StepOutput {
	if len(steps) == 0 {
		return nil
	}
	out := make([]StepOutput, len(steps))
	for i, s := range steps {
		out[i] = StepOutput{
			Text:             s.Text,
			Reasoning:        s.Reasoning,
			ToolCalls:        fromEngineToolCalls(s.ToolCalls),
			ToolResults:      fromEngineToolResults(s.ToolResults),
			FinishReason:     FinishReason(s.FinishReason),
			RawFinishReason:  s.RawFinishReason,
			ProviderMetadata: s.ProviderMetadata,
			Warnings:         fromEngineWarnings(s.Warnings),
		}
		if s.Usage != nil {
			out[i].Usage = fromEngineUsage(*s.Usage)
		}
		out[i].Response = Response{Messages: ResponseMessagesForStep(out[i], tools)}
	}
	return out
}

func fromEngineToolCallError(err error) error {
	var noSuchToolErr *engine.NoSuchToolError
	if errors.As(err, &noSuchToolErr) {
		if noSuchToolErr == nil {
			return nil
		}
		available := append([]string(nil), noSuchToolErr.AvailableTools...)
		return &NoSuchToolError{
			ToolName:       noSuchToolErr.ToolName,
			AvailableTools: available,
		}
	}

	var invalidArgsErr *engine.InvalidToolArgumentsError
	if errors.As(err, &invalidArgsErr) {
		if invalidArgsErr == nil {
			return nil
		}
		return &InvalidToolArgumentsError{
			ToolName: invalidArgsErr.ToolName,
			Args:     invalidArgsErr.Args,
			Cause:    invalidArgsErr.Cause,
		}
	}

	return err
}

func toChunkEvent(ev engine.StepEvent) ChunkEvent {
	var typ string
	switch ev.Type {
	case engine.StepEventTextDelta:
		typ = "text-delta"
	case engine.StepEventReasoningDelta:
		typ = "reasoning-delta"
	case engine.StepEventToolCallStart:
		typ = "tool-call-start"
	case engine.StepEventToolCallDelta:
		typ = "tool-call-delta"
	case engine.StepEventToolCallReady:
		typ = "tool-call-ready"
	case engine.StepEventToolResult:
		typ = "tool-result"
	case engine.StepEventUsage:
		typ = "usage"
	case engine.StepEventStepStart:
		typ = "step-start"
	case engine.StepEventStepEnd:
		typ = "step-end"
	case engine.StepEventDone:
		typ = "done"
	case engine.StepEventError:
		typ = "error"
	case engine.StepEventSource:
		typ = "source"
	case engine.StepEventFileDelta:
		typ = "file-delta"
	default:
		typ = "unknown"
	}
	return ChunkEvent{
		Type:              typ,
		TextDelta:         ev.TextDelta,
		ReasoningDelta:    ev.ReasoningDelta,
		ToolCallID:        ev.ToolCallID,
		ToolCallName:      ev.ToolCallName,
		ToolCallArgsDelta: ev.ToolCallArgsDelta,
		StepNumber:        ev.StepNumber,
		FinishReason:      FinishReason(ev.FinishReason),
	}
}

// engineModelAdapter wraps an ai.LanguageModel to satisfy engine.Model.
type engineModelAdapter struct {
	m LanguageModel
}

func (a *engineModelAdapter) ModelID() string { return a.m.ModelID() }

func (a *engineModelAdapter) Stream(ctx context.Context, req engine.Request) (<-chan engine.StreamEvent, error) {
	aiReq := LanguageModelRequest{
		Instructions:    req.Instructions,
		Messages:        fromEngineMessages(req.Messages),
		ProviderOptions: req.ProviderOptions,
		Settings: CallSettings{
			Temperature:   req.Settings.Temperature,
			MaxTokens:     req.Settings.MaxTokens,
			TopP:          req.Settings.TopP,
			TopK:          req.Settings.TopK,
			Seed:          req.Settings.Seed,
			StopSequences: req.Settings.StopSequences,
		},
	}
	if req.Output != nil {
		aiReq.Output = &OutputSchema{Type: req.Output.Type, Schema: req.Output.Schema}
	}
	if req.ToolChoice != nil {
		aiReq.ToolChoice = &ToolChoice{Type: req.ToolChoice.Type, ToolName: req.ToolChoice.ToolName}
	}
	for _, td := range req.Tools {
		aiReq.Tools = append(aiReq.Tools, ToolDefinition{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: td.InputSchema,
		})
	}

	aiCh, err := a.m.Stream(ctx, aiReq)
	if err != nil {
		return nil, err
	}

	engCh := make(chan engine.StreamEvent, 64)
	go func() {
		defer close(engCh)
		// A panic while adapting events surfaces as an error event before
		// close instead of crashing the process.
		defer safego.Recover(nil, func(err error) {
			select {
			case engCh <- engine.StreamEvent{Type: engine.StreamEventError, Error: err}:
			default:
			}
		})
		for ev := range aiCh {
			engCh <- toEngineStreamEvent(ev)
		}
	}()
	return engCh, nil
}

func toEngineStreamEvent(ev StreamEvent) engine.StreamEvent {
	e := engine.StreamEvent{
		Type:              engine.StreamEventType(ev.Type),
		TextDelta:         ev.TextDelta,
		ToolCallIndex:     ev.ToolCallIndex,
		ToolCallID:        ev.ToolCallID,
		ToolCallName:      ev.ToolCallName,
		ToolCallArgsDelta: ev.ToolCallArgsDelta,
		ThoughtSignature:  ev.ThoughtSignature,
		FinishReason:      engine.FinishReason(ev.FinishReason),
		RawFinishReason:   ev.RawFinishReason,
		ProviderMetadata:  ev.ProviderMetadata,
		FileData:          ev.FileData,
		FileMediaType:     ev.FileMediaType,
		Error:             ev.Error,
	}
	if ev.Usage != nil {
		e.Usage = &engine.Usage{
			InputTokens:  ev.Usage.InputTokens,
			OutputTokens: ev.Usage.OutputTokens,
			TotalTokens:  ev.Usage.TotalTokens,
			InputTokenDetails: engine.InputTokenDetails{
				NoCacheTokens:    ev.Usage.InputTokenDetails.NoCacheTokens,
				CacheReadTokens:  ev.Usage.InputTokenDetails.CacheReadTokens,
				CacheWriteTokens: ev.Usage.InputTokenDetails.CacheWriteTokens,
			},
			OutputTokenDetails: engine.OutputTokenDetails{
				TextTokens:      ev.Usage.OutputTokenDetails.TextTokens,
				ReasoningTokens: ev.Usage.OutputTokenDetails.ReasoningTokens,
			},
		}
	}
	if len(ev.Warnings) > 0 {
		e.Warnings = make([]engine.Warning, len(ev.Warnings))
		for i, w := range ev.Warnings {
			e.Warnings[i] = engine.Warning{Type: w.Type, Message: w.Message, Setting: w.Setting}
		}
	}
	if ev.Source != nil {
		e.Source = &engine.Source{
			SourceType:       ev.Source.SourceType,
			ID:               ev.Source.ID,
			URL:              ev.Source.URL,
			Title:            ev.Source.Title,
			ProviderMetadata: ev.Source.ProviderMetadata,
		}
	}
	return e
}

func toEngineMessages(msgs []Message) []engine.Message {
	out := make([]engine.Message, len(msgs))
	for i, m := range msgs {
		out[i] = engine.Message{
			Role:    string(m.Role),
			Content: toEngineContentParts(m.Content),
		}
	}
	return out
}

func toEngineContentParts(parts []ContentPart) []engine.ContentPart {
	out := make([]engine.ContentPart, len(parts))
	for i, p := range parts {
		ep := engine.ContentPart{
			Type:             string(p.Type),
			FileURL:          p.FileURL,
			MediaType:        p.MediaType,
			Data:             p.Data,
			FileID:           p.FileID,
			Filename:         p.Filename,
			ToolCallID:       p.ToolCallID,
			ToolCallName:     p.ToolCallName,
			ToolResultID:     p.ToolResultID,
			ToolResultName:   p.ToolResultName,
			ToolResultOutput: p.ToolResultOutput,
		}
		// "reasoning" parts store their text in ReasoningText; map to the engine's Text field.
		if p.Type == ContentPartTypeReasoning {
			ep.Text = p.ReasoningText
		} else {
			ep.Text = p.Text
		}
		if p.ToolCallArgs != nil {
			ep.ToolCallArgs = string(p.ToolCallArgs)
		}
		ep.ThoughtSignature = p.ThoughtSignature
		out[i] = ep
	}
	return out
}

func fromEngineMessages(msgs []engine.Message) []Message {
	out := make([]Message, len(msgs))
	for i, m := range msgs {
		out[i] = Message{
			Role:    Role(m.Role),
			Content: fromEngineContentParts(m.Content),
		}
	}
	return out
}

func fromEngineWarnings(ws []engine.Warning) []Warning {
	if len(ws) == 0 {
		return nil
	}
	out := make([]Warning, len(ws))
	for i, w := range ws {
		out[i] = Warning{Type: w.Type, Message: w.Message, Setting: w.Setting}
	}
	return out
}

func fromEngineContentParts(parts []engine.ContentPart) []ContentPart {
	out := make([]ContentPart, len(parts))
	for i, p := range parts {
		cp := ContentPart{
			Type:             ContentPartType(p.Type),
			FileURL:          p.FileURL,
			MediaType:        p.MediaType,
			Data:             p.Data,
			FileID:           p.FileID,
			Filename:         p.Filename,
			ToolCallID:       p.ToolCallID,
			ToolCallName:     p.ToolCallName,
			ToolResultID:     p.ToolResultID,
			ToolResultName:   p.ToolResultName,
			ToolResultOutput: p.ToolResultOutput,
		}
		// "reasoning" parts map the engine's Text field back to ReasoningText.
		if p.Type == "reasoning" {
			cp.ReasoningText = p.Text
		} else {
			cp.Text = p.Text
		}
		if p.ToolCallArgs != "" {
			cp.ToolCallArgs = json.RawMessage(p.ToolCallArgs)
		}
		cp.ThoughtSignature = p.ThoughtSignature
		out[i] = cp
	}
	return out
}
