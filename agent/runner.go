package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"sync/atomic"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/internal/jsonclone"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

var (
	ErrInvalidRun    = errors.New("agent: invalid run")
	ErrStreamUsed    = errors.New("agent: stream iterator already used")
	ErrHookStopped   = errors.New("agent: hook stopped run")
	errEmptyPrompt   = errors.New("prompt must not be empty")
	errEmptyMessages = errors.New("at least one input message is required")
)

// RunError reports invalid per-invocation configuration.
type RunError struct {
	Field string
	Err   error
}

func (e *RunError) Error() string {
	if e == nil {
		return ErrInvalidRun.Error()
	}
	if e.Field == "" {
		return fmt.Sprintf("%v: %v", ErrInvalidRun, e.Err)
	}
	return fmt.Sprintf("%v: %s: %v", ErrInvalidRun, e.Field, e.Err)
}

func (e *RunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Runner is a value-style builder for one Agent invocation.
type Runner struct {
	config   config
	messages []aikit.Message
	err      error
}

// Runner snapshots the Agent defaults for one invocation.
func (a *Agent) Runner() Runner {
	if a == nil {
		return Runner{err: &RunError{Field: "Agent", Err: errors.New("agent is nil")}}
	}
	return Runner{config: cloneConfig(a.config)}
}

// Messages replaces the complete ordered input sequence.
func (r Runner) Messages(messages ...aikit.Message) Runner {
	r.messages = cloneMessages(messages)
	return r
}

// Message appends one message to the ordered input sequence.
func (r Runner) Message(message aikit.Message) Runner {
	r.messages = append(cloneMessages(r.messages), message.Clone())
	return r
}

// Prompt appends a text user message.
func (r Runner) Prompt(prompt string) Runner {
	if prompt == "" && r.err == nil {
		r.err = &RunError{Field: "Prompt", Err: errEmptyPrompt}
		return r
	}
	return r.Message(aikit.UserMessage(prompt))
}

func (r Runner) Instructions(value string) Runner {
	r.config.instructions = value
	return r
}

func (r Runner) Tools(value *tool.Set) Runner {
	r.config.tools = cloneToolSet(value)
	return r
}

func (r Runner) ToolChoice(value aikit.ToolChoice) Runner {
	r.config.toolChoice = cloneToolChoice(&value)
	return r
}

func (r Runner) ActiveTools(names ...string) Runner {
	r.config.activeTools = append([]string{}, names...)
	return r
}

func (r Runner) MaxTurns(value int) Runner {
	r.config.maxTurns = value
	return r
}

func (r Runner) StopWhen(value aikit.StopCondition) Runner {
	r.config.stopWhen = value
	return r
}

func (r Runner) Output(value llm.OutputSchema) Runner {
	r.config.output = cloneOutputSchema(&value)
	return r
}

// OutputMode overrides the agent's structured-output mode for this run.
func (r Runner) OutputMode(value OutputMode) Runner {
	r.config.outputMode = value
	return r
}

func (r Runner) Settings(value llm.CallSettings) Runner {
	r.config.settings = cloneCallSettings(value)
	return r
}

func (r Runner) Temperature(value float32) Runner {
	r.config.settings.Temperature = clonePointer(&value)
	return r
}

func (r Runner) MaxTokens(value int) Runner {
	r.config.settings.MaxTokens = value
	return r
}

func (r Runner) TopP(value float32) Runner {
	r.config.settings.TopP = clonePointer(&value)
	return r
}

func (r Runner) TopK(value int) Runner {
	r.config.settings.TopK = clonePointer(&value)
	return r
}

func (r Runner) Seed(value int) Runner {
	r.config.settings.Seed = clonePointer(&value)
	return r
}

func (r Runner) StopSequences(values ...string) Runner {
	r.config.settings.StopSequences = append([]string(nil), values...)
	return r
}

func (r Runner) ProviderOptions(value map[string]any) Runner {
	r.config.providerOptions = jsonclone.Map(value)
	return r
}

func (r Runner) ProviderOptionsJSON(provider string, value map[string]any) Runner {
	r.config.providerOptions = jsonclone.Map(r.config.providerOptions)
	if r.config.providerOptions == nil {
		r.config.providerOptions = make(map[string]any)
	}
	r.config.providerOptions[provider] = jsonclone.Map(value)
	return r
}

func (r Runner) With(option llm.ProviderOption) Runner {
	if llm.IsNilProviderOption(option) {
		return r
	}
	r.config.providerOptions = jsonclone.Map(r.config.providerOptions)
	if r.config.providerOptions == nil {
		r.config.providerOptions = make(map[string]any)
	}
	r.config.providerOptions[option.ProviderName()] = option
	return r
}

func (r Runner) PrepareStep(value llm.PrepareStepFunc) Runner {
	r.config.prepareStep = value
	return r
}

func (r Runner) RepairToolCall(value aikit.RepairToolCallFunc) Runner {
	r.config.repairToolCall = value
	return r
}

func (r Runner) ToolsContext(value aikit.ToolsContext) Runner {
	r.config.toolsContext = cloneMap(value)
	return r
}

func (r Runner) RuntimeContext(value aikit.RuntimeContext) Runner {
	r.config.runtimeContext = cloneMap(value)
	return r
}

func (r Runner) ToolApproval(value map[string]ApprovalPolicy) Runner {
	r.config.toolApproval = cloneMap(value)
	return r
}

func (r Runner) ApprovalKey(value []byte) Runner {
	r.config.approvalKey = append([]byte(nil), value...)
	return r
}

func (r Runner) ApprovalReplayGuard(value ApprovalReplayGuard) Runner {
	r.config.approvalReplayGuard = value
	return r
}

func (r Runner) Approver(value ApprovalResponder) Runner {
	r.config.approver = value
	return r
}

func (r Runner) ToolConcurrency(value int) Runner {
	r.config.maxParallelTools = value
	r.config.parallelToolExecution = value > 1
	return r
}

func (r Runner) Logger(value *slog.Logger) Runner {
	r.config.logger = value
	return r
}

func (r Runner) Tracer(value Tracer) Runner {
	r.config.tracer = value
	return r
}

func (r Runner) TraceContent(value bool) Runner {
	r.config.traceContent = value
	return r
}

func (r Runner) Hook(value Hook) Runner {
	r.config.hooks = append(append([]Hook(nil), r.config.hooks...), value)
	return r
}

// Run executes and aggregates the same event driver exposed by Stream.
func (r Runner) Run(ctx context.Context) (*Result, error) {
	stream, err := r.newStepStream(ctx, false)
	if err != nil {
		return nil, err
	}
	for _, streamErr := range stream.Events() {
		if streamErr != nil {
			return stream.reducer.result, streamErr
		}
	}
	return stream.reducer.result, nil
}

// Stream validates the run and returns a single-use, single-owner event
// iterator. Breaking iteration cancels and drains the underlying runtime.
//
// It is StreamRun without the aggregate; use StreamRun when the run's *Result
// is wanted alongside its events.
func (r Runner) Stream(ctx context.Context) (iter.Seq2[aikit.StepEvent, error], error) {
	stream, err := r.StreamRun(ctx)
	if err != nil {
		return nil, err
	}
	return stream.Events(), nil
}

// StreamRun validates the run and returns its event sequence together with the
// Result that sequence aggregates. The aggregate costs nothing extra: the same
// reducer already ran behind Stream and was discarded.
func (r Runner) StreamRun(ctx context.Context) (*StepStream, error) {
	return r.newStepStream(ctx, true)
}

func (r Runner) newStepStream(ctx context.Context, streaming bool) (*StepStream, error) {
	if ctx == nil {
		return nil, &RunError{Field: "Context", Err: errors.New("context is nil")}
	}
	if err := r.validate(); err != nil {
		return nil, err
	}
	runID := newRunID()
	params, err := r.runParams(ctx, runID, streaming)
	if err != nil {
		return nil, err
	}
	stream := &StepStream{reducer: newResultReducer(r.initialTranscript(), r.config.tools)}
	stream.events = r.sequence(ctx, params, stream)
	return stream, nil
}

func (r Runner) sequence(
	ctx context.Context,
	params runConfig,
	stream *StepStream,
) iter.Seq2[aikit.StepEvent, error] {
	var used atomic.Bool
	reducer := stream.reducer
	return func(yield func(aikit.StepEvent, error) bool) {
		if !used.CompareAndSwap(false, true) {
			// A second range must not overwrite the first range's terminal
			// state; Result still answers for the run that happened.
			yield(aikit.StepEvent{}, ErrStreamUsed)
			return
		}
		child, cancel := context.WithCancel(ctx)
		ch := driveStream(child, params)
		hookContext := params.HookContext
		var runErr error
		completed := false
		defer func() {
			cancel()
			stream.err = runErr
			if completed {
				stream.state = StreamCompleted
			} else {
				stream.state = StreamAborted
			}
			r.notifyFinished(ctx, hookContext, reducer.result, runErr)
		}()
		sawDone := false
		for event := range ch {
			if event.Type == aikit.StepEventStepStart {
				hookContext.Turn = event.StepNumber + 1
			}
			if event.Type == aikit.StepEventError {
				streamErr := event.Error
				var maxTurns *MaxTurnsError
				if errors.As(streamErr, &maxTurns) {
					maxTurns.Result = reducer.result
				}
				runErr = streamErr
				yield(aikit.StepEvent{}, streamErr)
				cancel()
				for range ch {
				}
				return
			}
			if _, consumeErr := reducer.consume(event); consumeErr != nil {
				runErr = consumeErr
				yield(aikit.StepEvent{}, consumeErr)
				cancel()
				for range ch {
				}
				return
			}
			sawDone = sawDone || event.Type == aikit.StepEventDone
			if !yield(snapshotStepEvent(event), nil) {
				// Stopping on StepEventDone is a normal early exit with a whole
				// aggregate. Reporting it as a cancellation would hand Result a
				// failure that did not happen.
				if sawDone {
					completed = true
				} else {
					runErr = context.Canceled
				}
				cancel()
				for range ch {
				}
				return
			}
		}
		completed = true
	}
}

func (r Runner) validate() error {
	if r.err != nil {
		return r.err
	}
	if isNilInterface(r.config.model) {
		return &RunError{Field: "Model", Err: errNilModel}
	}
	if r.config.maxTurns < 1 {
		return &RunError{Field: "MaxTurns", Err: errInvalidMaxTurns}
	}
	if r.config.maxParallelTools < 1 {
		return &RunError{Field: "ToolConcurrency", Err: errInvalidConcurrency}
	}
	if len(r.messages) == 0 {
		return &RunError{Field: "Messages", Err: errEmptyMessages}
	}
	if r.config.tools != nil {
		if err := r.config.tools.Validate(); err != nil {
			return &RunError{Field: "Tools", Err: err}
		}
	}
	for i, hook := range r.config.hooks {
		if isNilInterface(hook) {
			return &RunError{Field: fmt.Sprintf("Hooks[%d]", i), Err: errNilHook}
		}
	}
	for i, message := range r.messages {
		if err := message.Validate(); err != nil {
			return &RunError{Field: fmt.Sprintf("Messages[%d]", i), Err: err}
		}
	}
	if err := validateMessageOrder(r.messages); err != nil {
		return &RunError{Field: "Messages", Err: err}
	}
	if err := validateActiveTools(r.config); err != nil {
		return &RunError{Field: "ActiveTools", Err: err}
	}
	if err := validateOutputConfiguration(r.config.output); err != nil {
		return &RunError{Field: "Output", Err: err}
	}
	for name, policy := range r.config.toolApproval {
		if policy == nil {
			return &RunError{Field: "ToolApproval[" + name + "]", Err: errNilApprovalPolicy}
		}
		if r.config.tools == nil {
			return &RunError{Field: "ToolApproval[" + name + "]", Err: errUnknownActiveTool}
		}
		if _, exists := r.config.tools.Lookup(name); !exists {
			return &RunError{Field: "ToolApproval[" + name + "]", Err: errUnknownActiveTool}
		}
	}
	return nil
}

func validateMessageOrder(messages []aikit.Message) error {
	toolCalls := make(map[string]string)
	approvals := make(map[string]string)
	for _, message := range messages {
		for _, part := range message.Content {
			switch part.Type {
			case aikit.ContentPartTypeToolCall:
				toolCalls[part.ToolCallID] = part.ToolCallName
				if part.ToolApprovalID != "" {
					approvals[part.ToolApprovalID] = part.ToolCallID
				}
			case aikit.ContentPartTypeToolResult:
				name, ok := toolCalls[part.ToolResultID]
				if !ok {
					return fmt.Errorf("tool result %q has no preceding tool call", part.ToolResultID)
				}
				if name != part.ToolResultName {
					return fmt.Errorf(
						"tool result %q names %q, preceding call names %q",
						part.ToolResultID,
						part.ToolResultName,
						name,
					)
				}
			case aikit.ContentPartTypeToolApprovalResponse:
				if _, ok := approvals[part.ToolApprovalID]; !ok {
					return fmt.Errorf("approval response %q has no preceding approval request", part.ToolApprovalID)
				}
			}
		}
	}
	return nil
}

func (r Runner) runParams(ctx context.Context, runID string, streaming bool) (runConfig, error) {
	definitions := []aikit.ToolDefinition(nil)
	if r.config.tools != nil {
		definitions = r.config.tools.DefinitionsSnapshot()
		if r.config.activeTools != nil {
			active := make(map[string]struct{}, len(r.config.activeTools))
			for _, name := range r.config.activeTools {
				active[name] = struct{}{}
			}
			filtered := definitions[:0]
			for _, definition := range definitions {
				if _, ok := active[definition.Name]; ok {
					filtered = append(filtered, definition)
				}
			}
			definitions = filtered
		}
	}
	stopWhen := r.config.stopWhen
	if stopWhen == nil {
		stopWhen = Never()
	}
	approval := make(map[string]func(string, string) bool, len(r.config.toolApproval))
	for name, policy := range r.config.toolApproval {
		approval[name] = policy
	}
	request := llm.Request{
		Instructions: r.config.instructions, Messages: cloneMessages(r.messages),
		Tools: definitions, ToolChoice: cloneToolChoice(r.config.toolChoice),
		Output: cloneOutputSchema(r.config.output), Settings: cloneCallSettings(r.config.settings),
		ProviderOptions: jsonclone.Map(r.config.providerOptions),
		ToolsContext:    cloneMap(r.config.toolsContext), RuntimeContext: cloneMap(r.config.runtimeContext),
	}
	// Existing custom models predate capability reporting and historically
	// received the native schema, so retain that behavior until they opt in.
	native := llm.NativeSchemaFull
	if capable, ok := r.config.model.(llm.NativeSchemaCapable); ok {
		native = capable.NativeSchemaSupport()
	}
	mode, err := resolveOutputMode(
		r.config.outputMode,
		r.config.output != nil && r.config.output.Type != "text",
		len(definitions) > 0,
		outputToolCallable(r.config.toolChoice, "structured_output"),
		native,
	)
	if err != nil {
		return runConfig{}, &RunError{Field: "OutputMode", Err: err}
	}
	// Gemini native cannot carry responseSchema beside function declarations.
	if mode == OutputModeNative && native == llm.NativeSchemaSuppressesTools && len(definitions) > 0 {
		request.Output = nil
	}
	return runConfig{
		Model: r.config.model, Request: request, Tools: r.config.tools,
		OutputMode: mode,
		StopWhen:   stopWhen, MaxSteps: r.config.maxTurns,
		ErrorOnMaxTurns: true,
		PrepareStep:     r.config.prepareStep, RepairToolCall: r.config.repairToolCall,
		ToolApproval: approval, ApprovalKey: append([]byte(nil), r.config.approvalKey...),
		ApprovalReplayGuard: r.config.approvalReplayGuard, Approver: r.config.approver,
		ParallelToolExecution: r.config.parallelToolExecution,
		MaxParallelTools:      r.config.maxParallelTools, Logger: r.config.logger,
		Tracer: r.config.tracer, TraceContent: r.config.traceContent,
		Hooks: append([]Hook(nil), r.config.hooks...),
		HookContext: HookContext{
			RunID: runID, Streaming: streaming, AgentID: r.config.id, scratchpad: newScratchpad(),
		},
	}, nil
}

func applyRequestPatch(request llm.Request, patch RequestPatch) llm.Request {
	if patch.Instructions != nil {
		request.Instructions = *patch.Instructions
	}
	if patch.Messages != nil {
		request.Messages = cloneMessages(patch.Messages)
	}
	if patch.ProviderOptions != nil {
		if request.ProviderOptions == nil {
			request.ProviderOptions = make(map[string]any)
		}
		for key, value := range patch.ProviderOptions {
			request.ProviderOptions[key] = jsonclone.Value(value)
		}
	}
	if patch.Settings != nil {
		request.Settings = cloneCallSettings(*patch.Settings)
	}
	if patch.ToolChoice != nil {
		request.ToolChoice = cloneToolChoice(patch.ToolChoice)
	}
	if patch.ActiveTools != nil {
		active := make(map[string]struct{}, len(patch.ActiveTools))
		for _, name := range patch.ActiveTools {
			active[name] = struct{}{}
		}
		filtered := request.Tools[:0]
		for _, definition := range request.Tools {
			if _, ok := active[definition.Name]; ok {
				filtered = append(filtered, definition)
			}
		}
		request.Tools = filtered
	}
	return request
}

func cloneRequest(request llm.Request) llm.Request {
	request.Messages = cloneMessages(request.Messages)
	request.Tools = append([]aikit.ToolDefinition(nil), request.Tools...)
	request.ToolChoice = cloneToolChoice(request.ToolChoice)
	request.Output = cloneOutputSchema(request.Output)
	request.Settings = cloneCallSettings(request.Settings)
	request.ProviderOptions = jsonclone.Map(request.ProviderOptions)
	request.ToolsContext = cloneMap(request.ToolsContext)
	request.RuntimeContext = cloneMap(request.RuntimeContext)
	return request
}

func (r Runner) initialTranscript() []aikit.Message {
	messages := make([]aikit.Message, 0, len(r.messages)+1)
	if r.config.instructions != "" {
		messages = append(messages, aikit.SystemMessage(r.config.instructions))
	}
	return append(messages, cloneMessages(r.messages)...)
}

func (r Runner) notifyFinished(ctx context.Context, hookContext HookContext, result *Result, runErr error) {
	for _, hook := range r.config.hooks {
		if capability, ok := hook.(RunFinishedHook); ok {
			func() {
				defer func() {
					if value := recover(); value != nil && r.config.logger != nil {
						r.config.logger.Error(
							"agent run-finished hook panicked",
							"hook", hook.HookName(), "panic", value,
						)
					}
				}()
				capability.OnRunFinished(ctx, hookContext, cloneResult(result), runErr)
			}()
		}
	}
}

func newRunID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(value[:])
}
