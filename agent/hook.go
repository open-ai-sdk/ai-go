package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// Hook is the minimal identity shared by optional lifecycle capabilities.
// Implement only the capability interfaces needed by a hook.
type Hook interface {
	HookName() string
}

// HookContext is stable for one run and is passed by value to hooks.
type HookContext struct {
	RunID      string
	Turn       int
	Streaming  bool
	AgentID    string
	scratchpad *scratchpad
}

// HookError identifies a lifecycle hook failure while preserving its cause.
// Phase is one of before_completion, before_tool, after_tool,
// invalid_tool_call, or stream_event.
type HookError struct {
	Hook  string
	Phase string
	Err   error
}

func (e *HookError) Error() string {
	if e == nil {
		return "agent: hook failed"
	}
	return fmt.Sprintf("agent: hook %q %s: %v", e.Hook, e.Phase, e.Err)
}

func (e *HookError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RequestPatch applies only to the current model turn.
type RequestPatch struct {
	Instructions    *string
	Messages        []aikit.Message
	ProviderOptions map[string]any
	ActiveTools     []string
	Settings        *llm.CallSettings
	ToolChoice      *aikit.ToolChoice
}

// ToolResultEvent presents immutable execution facts separately from the
// running model presentation. Rewriters may change Presentation only.
type ToolResultEvent struct {
	Raw          aikit.ToolResult
	Presentation aikit.ToolResult
}

// CompletionResponseEvent is the canonical response produced by one provider
// call before tool dispatch or finalization.
type CompletionResponseEvent struct {
	Text, Reasoning string
	MessageID       string
	Usage           aikit.Usage
	FinishReason    aikit.FinishReason
}

// ModelTurnEvent is the completed model turn parked for hook acceptance.
// Retry is valid only when HasToolCalls is false.
type ModelTurnEvent struct {
	CompletionResponseEvent
	HasToolCalls bool
}

// TextDeltaEvent provides both the newest fragment and the turn aggregate.
type TextDeltaEvent struct{ Delta, Text string }

// ToolCallDeltaEvent describes a streamed tool-call argument fragment.
type ToolCallDeltaEvent struct {
	ID, Name string
	Index    int
	Delta    json.RawMessage
}

// StreamFinishEvent is emitted after the provider finishes a stream and
// before model-turn acceptance.
type StreamFinishEvent struct{ CompletionResponseEvent }

// ObservationAction controls observer-only lifecycle events.
type ObservationActionKind uint8

const (
	ObservationContinue ObservationActionKind = iota
	ObservationStop
)

type ObservationAction struct {
	Kind   ObservationActionKind
	Reason string
}

// RetryRequest describes how a tool-free rejected model turn should continue.
type RetryRequest struct{ Feedback string }

func Repeat() RetryRequest                           { return RetryRequest{} }
func RetryWithFeedback(feedback string) RetryRequest { return RetryRequest{Feedback: feedback} }

type ModelTurnActionKind uint8

const (
	ModelTurnContinue ModelTurnActionKind = iota
	ModelTurnRetry
	ModelTurnStop
)

// ModelTurnAction accepts, retries, or stops a canonical model turn.
type ModelTurnAction struct {
	Kind   ModelTurnActionKind
	Retry  RetryRequest
	Reason string
}

// HookInterest is an optional high-frequency observation hint.
type HookInterest uint8

const (
	HookInterestTextDelta HookInterest = 1 << iota
	HookInterestToolCallDelta
	HookInterestStreamFinish
)

// ToolResultHook is the rich post-tool capability. AfterTool remains
// supported for existing pre-release hooks.
type ToolResultHook interface {
	Hook
	OnToolResult(context.Context, HookContext, ToolResultEvent) (ToolResultAction, error)
}
type CompletionResponseHook interface {
	Hook
	OnCompletionResponse(context.Context, HookContext, CompletionResponseEvent) (ObservationAction, error)
}
type ModelTurnHook interface {
	Hook
	OnModelTurn(context.Context, HookContext, ModelTurnEvent) (ModelTurnAction, error)
}
type TextDeltaHook interface {
	Hook
	OnTextDelta(context.Context, HookContext, TextDeltaEvent) (ObservationAction, error)
}
type ToolCallDeltaHook interface {
	Hook
	OnToolCallDelta(context.Context, HookContext, ToolCallDeltaEvent) (ObservationAction, error)
}
type StreamFinishHook interface {
	Hook
	OnStreamFinish(context.Context, HookContext, StreamFinishEvent) (ObservationAction, error)
}
type InterestedHook interface {
	Hook
	HookInterests() HookInterest
}

type CompletionActionKind uint8

const (
	CompletionContinue CompletionActionKind = iota
	CompletionPatch
	CompletionStop
)

type CompletionAction struct {
	Kind   CompletionActionKind
	Patch  RequestPatch
	Reason string
}

type ToolCallActionKind uint8

const (
	ToolCallRun ToolCallActionKind = iota
	ToolCallRewrite
	ToolCallSkip
	ToolCallStop
)

type ToolCallAction struct {
	Kind   ToolCallActionKind
	Args   json.RawMessage
	Reason string
}

type ToolResultActionKind uint8

const (
	ToolResultKeep ToolResultActionKind = iota
	ToolResultRewrite
	ToolResultStop
)

type ToolResultAction struct {
	Kind   ToolResultActionKind
	Result aikit.ToolResult
	Reason string
}

type InvalidToolCallActionKind uint8

const (
	// InvalidToolCallContinue lets the next registered recovery hook decide.
	InvalidToolCallContinue InvalidToolCallActionKind = iota
	InvalidToolCallFail
	InvalidToolCallRetry
	InvalidToolCallRepair
	InvalidToolCallSkip
	InvalidToolCallStop
)

type InvalidToolCallAction struct {
	Kind     InvalidToolCallActionKind
	Repaired *aikit.ToolCallInfo
	Reason   string
}

type BeforeCompletionHook interface {
	Hook
	BeforeCompletion(context.Context, HookContext, llm.Request) (CompletionAction, error)
}

type BeforeToolHook interface {
	Hook
	BeforeTool(context.Context, HookContext, aikit.ToolCallInfo) (ToolCallAction, error)
}

type AfterToolHook interface {
	Hook
	AfterTool(context.Context, HookContext, aikit.ToolResult) (ToolResultAction, error)
}

type InvalidToolCallHook interface {
	Hook
	InvalidToolCall(context.Context, HookContext, aikit.RepairToolCallInput) (InvalidToolCallAction, error)
}

type StreamEventHook interface {
	Hook
	OnStreamEvent(context.Context, HookContext, aikit.StepEvent) error
}

type RunFinishedHook interface {
	Hook
	OnRunFinished(context.Context, HookContext, *Result, error)
}

// HookFuncs is a convenience adapter for function-based hooks.
type HookFuncs struct {
	Name                   string
	BeforeCompletionFunc   func(context.Context, HookContext, llm.Request) (CompletionAction, error)
	BeforeToolFunc         func(context.Context, HookContext, aikit.ToolCallInfo) (ToolCallAction, error)
	AfterToolFunc          func(context.Context, HookContext, aikit.ToolResult) (ToolResultAction, error)
	ToolResultFunc         func(context.Context, HookContext, ToolResultEvent) (ToolResultAction, error)
	CompletionResponseFunc func(context.Context, HookContext, CompletionResponseEvent) (ObservationAction, error)
	ModelTurnFunc          func(context.Context, HookContext, ModelTurnEvent) (ModelTurnAction, error)
	TextDeltaFunc          func(context.Context, HookContext, TextDeltaEvent) (ObservationAction, error)
	ToolCallDeltaFunc      func(context.Context, HookContext, ToolCallDeltaEvent) (ObservationAction, error)
	StreamFinishFunc       func(context.Context, HookContext, StreamFinishEvent) (ObservationAction, error)
	InvalidToolCallFunc    func(context.Context, HookContext, aikit.RepairToolCallInput) (InvalidToolCallAction, error)
	StreamEventFunc        func(context.Context, HookContext, aikit.StepEvent) error
	RunFinishedFunc        func(context.Context, HookContext, *Result, error)
}

func (h HookFuncs) HookInterests() HookInterest {
	var value HookInterest
	if h.TextDeltaFunc != nil {
		value |= HookInterestTextDelta
	}
	if h.ToolCallDeltaFunc != nil {
		value |= HookInterestToolCallDelta
	}
	if h.StreamFinishFunc != nil {
		value |= HookInterestStreamFinish
	}
	return value
}

func (h HookFuncs) OnCompletionResponse(
	ctx context.Context,
	hc HookContext,
	event CompletionResponseEvent,
) (ObservationAction, error) {
	if h.CompletionResponseFunc == nil {
		return ObservationAction{Kind: ObservationContinue}, nil
	}
	return h.CompletionResponseFunc(ctx, hc, event)
}

func (h HookFuncs) OnModelTurn(ctx context.Context, hc HookContext, event ModelTurnEvent) (ModelTurnAction, error) {
	if h.ModelTurnFunc == nil {
		return ModelTurnAction{Kind: ModelTurnContinue}, nil
	}
	return h.ModelTurnFunc(ctx, hc, event)
}

func (h HookFuncs) OnTextDelta(ctx context.Context, hc HookContext, event TextDeltaEvent) (ObservationAction, error) {
	if h.TextDeltaFunc == nil {
		return ObservationAction{Kind: ObservationContinue}, nil
	}
	return h.TextDeltaFunc(ctx, hc, event)
}

func (h HookFuncs) OnToolCallDelta(
	ctx context.Context,
	hc HookContext,
	event ToolCallDeltaEvent,
) (ObservationAction, error) {
	if h.ToolCallDeltaFunc == nil {
		return ObservationAction{Kind: ObservationContinue}, nil
	}
	return h.ToolCallDeltaFunc(ctx, hc, event)
}

func (h HookFuncs) OnStreamFinish(
	ctx context.Context,
	hc HookContext,
	event StreamFinishEvent,
) (ObservationAction, error) {
	if h.StreamFinishFunc == nil {
		return ObservationAction{Kind: ObservationContinue}, nil
	}
	return h.StreamFinishFunc(ctx, hc, event)
}

func (h HookFuncs) OnToolResult(ctx context.Context, hc HookContext, event ToolResultEvent) (ToolResultAction, error) {
	if h.ToolResultFunc == nil {
		return ToolResultAction{Kind: ToolResultKeep}, nil
	}
	return h.ToolResultFunc(ctx, hc, ToolResultEvent{Raw: event.Raw.Clone(), Presentation: event.Presentation.Clone()})
}

func (h HookFuncs) HookName() string { return h.Name }

func (h HookFuncs) BeforeCompletion(
	ctx context.Context,
	hc HookContext,
	request llm.Request,
) (CompletionAction, error) {
	if h.BeforeCompletionFunc == nil {
		return CompletionAction{Kind: CompletionContinue}, nil
	}
	return h.BeforeCompletionFunc(ctx, hc, request)
}

func (h HookFuncs) BeforeTool(ctx context.Context, hc HookContext, call aikit.ToolCallInfo) (ToolCallAction, error) {
	if h.BeforeToolFunc == nil {
		return ToolCallAction{Kind: ToolCallRun}, nil
	}
	return h.BeforeToolFunc(ctx, hc, call)
}

func (h HookFuncs) AfterTool(ctx context.Context, hc HookContext, result aikit.ToolResult) (ToolResultAction, error) {
	if h.AfterToolFunc == nil {
		return ToolResultAction{Kind: ToolResultKeep}, nil
	}
	return h.AfterToolFunc(ctx, hc, result)
}

func (h HookFuncs) InvalidToolCall(
	ctx context.Context,
	hc HookContext,
	input aikit.RepairToolCallInput,
) (InvalidToolCallAction, error) {
	if h.InvalidToolCallFunc == nil {
		return InvalidToolCallAction{Kind: InvalidToolCallContinue}, nil
	}
	return h.InvalidToolCallFunc(ctx, hc, input)
}

func hookFailure(hook Hook, phase string, err error) error {
	if err == nil {
		return nil
	}
	return &HookError{Hook: hook.HookName(), Phase: phase, Err: err}
}

func hookStopped(hook Hook, phase, reason string) error {
	err := ErrHookStopped
	if reason != "" {
		err = fmt.Errorf("%w: %s", ErrHookStopped, reason)
	}
	return hookFailure(hook, phase, err)
}

func panicError(value interface{}) error {
	if err, ok := value.(error); ok {
		return err
	}
	return fmt.Errorf("panic: %v", value)
}

func callBeforeCompletion(
	hook BeforeCompletionHook,
	ctx context.Context,
	hc HookContext,
	request llm.Request,
) (action CompletionAction, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = panicError(value)
		}
	}()
	return hook.BeforeCompletion(ctx, hc, request)
}

func callBeforeTool(
	hook BeforeToolHook,
	ctx context.Context,
	hc HookContext,
	call aikit.ToolCallInfo,
) (action ToolCallAction, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = panicError(value)
		}
	}()
	return hook.BeforeTool(ctx, hc, call)
}

func callAfterTool(
	hook AfterToolHook,
	ctx context.Context,
	hc HookContext,
	result aikit.ToolResult,
) (action ToolResultAction, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = panicError(value)
		}
	}()
	return hook.AfterTool(ctx, hc, result)
}

func callToolResult(
	hook ToolResultHook,
	ctx context.Context,
	hc HookContext,
	event ToolResultEvent,
) (action ToolResultAction, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = panicError(value)
		}
	}()
	return hook.OnToolResult(ctx, hc, event)
}

func callInvalidTool(
	hook InvalidToolCallHook,
	ctx context.Context,
	hc HookContext,
	input aikit.RepairToolCallInput,
) (action InvalidToolCallAction, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = panicError(value)
		}
	}()
	return hook.InvalidToolCall(ctx, hc, input)
}

func callObservation(fn func() (ObservationAction, error)) (action ObservationAction, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = panicError(value)
		}
	}()
	return fn()
}

func callModelTurn(fn func() (ModelTurnAction, error)) (action ModelTurnAction, err error) {
	defer func() {
		if value := recover(); value != nil {
			err = panicError(value)
		}
	}()
	return fn()
}

func completionActionError(hook Hook, kind CompletionActionKind) error {
	return hookFailure(hook, "before_completion", fmt.Errorf("invalid action %d", kind))
}

func toolCallActionError(hook Hook, kind ToolCallActionKind) error {
	return hookFailure(hook, "before_tool", fmt.Errorf("invalid action %d", kind))
}

func toolResultActionError(hook Hook, kind ToolResultActionKind) error {
	return hookFailure(hook, "after_tool", fmt.Errorf("invalid action %d", kind))
}

func invalidToolActionError(hook Hook, kind InvalidToolCallActionKind) error {
	return hookFailure(hook, "invalid_tool_call", fmt.Errorf("invalid action %d", kind))
}

func (h HookFuncs) OnStreamEvent(ctx context.Context, hc HookContext, event aikit.StepEvent) error {
	if h.StreamEventFunc == nil {
		return nil
	}
	return h.StreamEventFunc(ctx, hc, event)
}

func (h HookFuncs) OnRunFinished(ctx context.Context, hc HookContext, result *Result, err error) {
	if h.RunFinishedFunc != nil {
		h.RunFinishedFunc(ctx, hc, result, err)
	}
}

func (r *run) beforeCompletion(request llm.Request) (llm.Request, error) {
	r.hookMu.Lock()
	defer r.hookMu.Unlock()
	if r.hookErr != nil {
		return llm.Request{}, r.hookErr
	}
	for _, hook := range r.hooks {
		capability, ok := hook.(BeforeCompletionHook)
		if !ok {
			continue
		}
		action, err := callBeforeCompletion(
			capability, r.ctx, r.hookContext, cloneRequest(request),
		)
		if err != nil {
			return llm.Request{}, hookFailure(hook, "before_completion", err)
		}
		switch action.Kind {
		case CompletionContinue:
		case CompletionPatch:
			request = applyRequestPatch(request, action.Patch)
		case CompletionStop:
			return llm.Request{}, hookStopped(hook, "before_completion", action.Reason)
		default:
			return llm.Request{}, completionActionError(hook, action.Kind)
		}
	}
	return request, nil
}

func (r *run) observeCompletionResponse(event CompletionResponseEvent) error {
	r.hookMu.Lock()
	defer r.hookMu.Unlock()
	for _, hook := range r.hooks {
		capability, ok := hook.(CompletionResponseHook)
		if !ok {
			continue
		}
		action, err := callObservation(func() (ObservationAction, error) {
			return capability.OnCompletionResponse(r.ctx, r.hookContext, event)
		})
		if err != nil {
			return hookFailure(hook, "completion_response", err)
		}
		if action.Kind == ObservationStop {
			return hookStopped(hook, "completion_response", action.Reason)
		}
		if action.Kind != ObservationContinue {
			return hookFailure(hook, "completion_response", fmt.Errorf("invalid observation action %d", action.Kind))
		}
	}
	return nil
}

func (r *run) modelTurn(event ModelTurnEvent) (ModelTurnAction, error) {
	for _, hook := range r.hooks {
		capability, ok := hook.(ModelTurnHook)
		if !ok {
			continue
		}
		action, err := callModelTurn(func() (ModelTurnAction, error) {
			return capability.OnModelTurn(r.ctx, r.hookContext, event)
		})
		if err != nil {
			return ModelTurnAction{}, hookFailure(hook, "model_turn", err)
		}
		switch action.Kind {
		case ModelTurnContinue:
		case ModelTurnRetry:
			if event.HasToolCalls {
				err := errors.New("retry is invalid for a turn containing tool calls")
				return ModelTurnAction{}, hookFailure(hook, "model_turn", err)
			}
			return action, nil
		case ModelTurnStop:
			return ModelTurnAction{}, hookStopped(hook, "model_turn", action.Reason)
		default:
			err := fmt.Errorf("invalid model turn action %d", action.Kind)
			return ModelTurnAction{}, hookFailure(hook, "model_turn", err)
		}
	}
	return ModelTurnAction{Kind: ModelTurnContinue}, nil
}

func (r *run) observeTextDelta(event TextDeltaEvent) error {
	return r.observeDelta(HookInterestTextDelta, "text_delta", func(h Hook) (ObservationAction, error) {
		capability, ok := h.(TextDeltaHook)
		if !ok {
			return ObservationAction{Kind: ObservationContinue}, nil
		}
		return callObservation(func() (ObservationAction, error) {
			return capability.OnTextDelta(r.ctx, r.hookContext, event)
		})
	})
}

func (r *run) observeToolCallDelta(event ToolCallDeltaEvent) error {
	return r.observeDelta(HookInterestToolCallDelta, "tool_call_delta", func(h Hook) (ObservationAction, error) {
		capability, ok := h.(ToolCallDeltaHook)
		if !ok {
			return ObservationAction{Kind: ObservationContinue}, nil
		}
		return callObservation(func() (ObservationAction, error) {
			return capability.OnToolCallDelta(r.ctx, r.hookContext, event)
		})
	})
}

func (r *run) observeStreamFinish(event StreamFinishEvent) error {
	return r.observeDelta(HookInterestStreamFinish, "stream_finish", func(h Hook) (ObservationAction, error) {
		capability, ok := h.(StreamFinishHook)
		if !ok {
			return ObservationAction{Kind: ObservationContinue}, nil
		}
		return callObservation(func() (ObservationAction, error) {
			return capability.OnStreamFinish(r.ctx, r.hookContext, event)
		})
	})
}

func (r *run) observeDelta(interest HookInterest, phase string, invoke func(Hook) (ObservationAction, error)) error {
	r.hookMu.Lock()
	defer r.hookMu.Unlock()
	for _, hook := range r.hooks {
		if interested, ok := hook.(InterestedHook); ok && interested.HookInterests()&interest == 0 {
			continue
		}
		action, err := invoke(hook)
		if err != nil {
			return hookFailure(hook, phase, err)
		}
		if action.Kind == ObservationStop {
			return hookStopped(hook, phase, action.Reason)
		}
		if action.Kind != ObservationContinue {
			return hookFailure(hook, phase, fmt.Errorf("invalid observation action %d", action.Kind))
		}
	}
	return nil
}

func (r *run) hasModelTurnHooks() bool {
	for _, hook := range r.hooks {
		switch value := hook.(type) {
		case HookFuncs:
			if value.ModelTurnFunc != nil {
				return true
			}
		case *HookFuncs:
			if value != nil && value.ModelTurnFunc != nil {
				return true
			}
		default:
			if _, ok := hook.(ModelTurnHook); ok {
				return true
			}
		}
	}
	return false
}

func (r *run) beforeTool(tc toolCallState) (toolCallState, bool, string, error) {
	r.hookMu.Lock()
	defer r.hookMu.Unlock()
	if r.hookErr != nil {
		return tc, false, "", r.hookErr
	}
	for _, hook := range r.hooks {
		capability, ok := hook.(BeforeToolHook)
		if !ok {
			continue
		}
		call := aikit.ToolCallInfo{
			ID: tc.id, Name: tc.name, Args: append(json.RawMessage(nil), tc.args...),
			ArgsSet: true, ThoughtSignature: tc.thoughtSignature,
		}
		action, err := callBeforeTool(capability, r.ctx, r.hookContext, call)
		if err != nil {
			return tc, false, "", hookFailure(hook, "before_tool", err)
		}
		switch action.Kind {
		case ToolCallRun:
		case ToolCallRewrite:
			tc.args = string(append(json.RawMessage(nil), action.Args...))
			if err := invalidToolArgumentsError(tc.name, tc.args); err != nil {
				return tc, false, "", hookFailure(hook, "before_tool", err)
			}
		case ToolCallSkip:
			return tc, true, action.Reason, nil
		case ToolCallStop:
			return tc, false, "", hookStopped(hook, "before_tool", action.Reason)
		default:
			return tc, false, "", toolCallActionError(hook, action.Kind)
		}
	}
	return tc, false, "", nil
}

func (r *run) afterTool(result *aikit.ToolResult) (*aikit.ToolResult, error) {
	if result == nil {
		return nil, nil
	}
	r.hookMu.Lock()
	defer r.hookMu.Unlock()
	if r.hookErr != nil {
		return nil, r.hookErr
	}
	raw := result.Clone()
	effective := result.Clone()
	for _, hook := range r.hooks {
		if capability, ok := hook.(ToolResultHook); ok {
			event := ToolResultEvent{Raw: raw.Clone(), Presentation: effective.Clone()}
			action, err := callToolResult(capability, r.ctx, r.hookContext, event)
			if err != nil {
				return nil, hookFailure(hook, "after_tool", err)
			}
			switch action.Kind {
			case ToolResultKeep:
			case ToolResultRewrite:
				rewritten := action.Result.Clone()
				if rewritten.ID == "" {
					rewritten.ID = effective.ID
				}
				if rewritten.Name == "" {
					rewritten.Name = effective.Name
				}
				if rewritten.Args == "" {
					rewritten.Args = effective.Args
				}
				effective = rewritten
			case ToolResultStop:
				return nil, hookStopped(hook, "after_tool", action.Reason)
			default:
				return nil, toolResultActionError(hook, action.Kind)
			}
		}
		capability, ok := hook.(AfterToolHook)
		if !ok {
			continue
		}
		action, err := callAfterTool(capability, r.ctx, r.hookContext, effective.Clone())
		if err != nil {
			return nil, hookFailure(hook, "after_tool", err)
		}
		switch action.Kind {
		case ToolResultKeep:
		case ToolResultRewrite:
			rewritten := action.Result.Clone()
			if rewritten.ID == "" {
				rewritten.ID = effective.ID
			}
			if rewritten.Name == "" {
				rewritten.Name = effective.Name
			}
			if rewritten.Args == "" {
				rewritten.Args = effective.Args
			}
			effective = rewritten
		case ToolResultStop:
			return nil, hookStopped(hook, "after_tool", action.Reason)
		default:
			return nil, toolResultActionError(hook, action.Kind)
		}
	}
	return &effective, nil
}

func (r *run) recoverInvalidToolCall(
	input aikit.RepairToolCallInput,
) (InvalidToolCallAction, error) {
	r.hookMu.Lock()
	defer r.hookMu.Unlock()
	if r.hookErr != nil {
		return InvalidToolCallAction{}, r.hookErr
	}
	for _, hook := range r.hooks {
		capability, ok := hook.(InvalidToolCallHook)
		if !ok {
			continue
		}
		action, err := callInvalidTool(capability, r.ctx, r.hookContext, input)
		if err != nil {
			return InvalidToolCallAction{}, hookFailure(hook, "invalid_tool_call", err)
		}
		switch action.Kind {
		case InvalidToolCallContinue:
			continue
		case InvalidToolCallFail, InvalidToolCallRetry, InvalidToolCallSkip:
			return action, nil
		case InvalidToolCallRepair:
			if action.Repaired == nil {
				return InvalidToolCallAction{}, hookFailure(
					hook, "invalid_tool_call", errors.New("repair action has no repaired call"),
				)
			}
			action.Repaired = cloneToolCallInfo(action.Repaired)
			return action, nil
		case InvalidToolCallStop:
			return InvalidToolCallAction{}, hookStopped(hook, "invalid_tool_call", action.Reason)
		default:
			return InvalidToolCallAction{}, invalidToolActionError(hook, action.Kind)
		}
	}
	return InvalidToolCallAction{Kind: InvalidToolCallFail}, nil
}

func cloneToolCallInfo(source *aikit.ToolCallInfo) *aikit.ToolCallInfo {
	if source == nil {
		return nil
	}
	cloned := *source
	cloned.Args = append(json.RawMessage(nil), source.Args...)
	return &cloned
}
