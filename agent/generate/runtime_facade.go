package generate

import "context"

// Option configures a single GenerateTextRequest field.
// Options are applied in order after the prompt is set.
type Option func(*GenerateTextRequest)

// WithInstructions sets the system prompt.
func WithInstructions(s string) Option {
	return func(r *GenerateTextRequest) { r.Instructions = s }
}

// WithMessages sets (or replaces) the conversation history.
func WithMessages(msgs ...Message) Option {
	return func(r *GenerateTextRequest) { r.Messages = msgs }
}

// WithTools attaches a ToolSet to the request.
func WithTools(ts *ToolSet) Option {
	return func(r *GenerateTextRequest) { r.Tools = ts }
}

// WithToolChoice sets the tool-selection policy.
func WithToolChoice(tc ToolChoice) Option {
	return func(r *GenerateTextRequest) { r.ToolChoice = &tc }
}

// WithMaxSteps limits the number of tool-loop iterations.
func WithMaxSteps(n int) Option {
	return func(r *GenerateTextRequest) { r.MaxSteps = n }
}

// WithTemperature sets the sampling temperature.
func WithTemperature(t float32) Option {
	return func(r *GenerateTextRequest) { r.Settings.Temperature = &t }
}

// WithMaxTokens limits completion tokens.
func WithMaxTokens(n int) Option {
	return func(r *GenerateTextRequest) { r.Settings.MaxTokens = n }
}

// WithTopP sets nucleus-sampling probability mass.
func WithTopP(p float32) Option {
	return func(r *GenerateTextRequest) { r.Settings.TopP = &p }
}

// WithTopK limits next-token candidates to the top K (provider support varies).
func WithTopK(k int) Option {
	return func(r *GenerateTextRequest) { r.Settings.TopK = &k }
}

// WithSeed requests deterministic sampling (provider support varies).
func WithSeed(seed int) Option {
	return func(r *GenerateTextRequest) { r.Settings.Seed = &seed }
}

// WithStopSequences stops generation when any of the given strings is produced.
func WithStopSequences(seqs ...string) Option {
	return func(r *GenerateTextRequest) { r.Settings.StopSequences = seqs }
}

// WithOutput constrains the model output to the given schema.
func WithOutput(o *OutputSchema) Option {
	return func(r *GenerateTextRequest) { r.Output = o }
}

// WithStopWhen sets a custom stop condition for the tool loop.
func WithStopWhen(sc StopCondition) Option {
	return func(r *GenerateTextRequest) { r.StopWhen = sc }
}

// WithProviderOptions attaches provider-specific options.
func WithProviderOptions(opts map[string]any) Option {
	return func(r *GenerateTextRequest) { r.ProviderOptions = opts }
}

func WithToolsContext(contexts ToolsContext) Option {
	return func(r *GenerateTextRequest) { r.ToolsContext = contexts }
}

func WithRuntimeContext(context RuntimeContext) Option {
	return func(r *GenerateTextRequest) { r.RuntimeContext = context }
}

func WithToolApproval(approval map[string]ToolApprovalFunc) Option {
	return func(r *GenerateTextRequest) { r.ToolApproval = approval }
}

// WithToolApprovalKey sets the HMAC key used for stateless approval
// suspension and resumption. The key must contain at least 32 random bytes.
func WithToolApprovalKey(key []byte) Option {
	return func(r *GenerateTextRequest) { r.ToolApprovalKey = append([]byte(nil), key...) }
}

func WithToolApprovalReplayGuard(guard ToolApprovalReplayGuard) Option {
	return func(r *GenerateTextRequest) { r.ToolApprovalReplayGuard = guard }
}

// WithModel overrides the model for this single call, ignoring the Runtime default.
func WithModel(m LanguageModel) Option {
	return func(r *GenerateTextRequest) { r.Model = m }
}

// WithSmoothStream enables smooth text streaming with the given configuration.
// Only effective with StreamText; ignored by GenerateText.
func WithSmoothStream(ss *SmoothStream) Option {
	return func(r *GenerateTextRequest) { r.SmoothStream = ss }
}

// WithRepairToolCall enables automatic repair for invalid tool calls.
func WithRepairToolCall(fn RepairToolCallFunc) Option {
	return func(r *GenerateTextRequest) { r.RepairToolCall = fn }
}

// WithParallelToolExecution enables parallel execution of tool calls within a step.
// By default, tool calls are executed sequentially.
func WithParallelToolExecution(enabled bool) Option {
	return func(r *GenerateTextRequest) { r.ParallelToolExecution = enabled }
}

// WithMaxParallelTools limits the number of concurrent tool executions (default
// 5 when parallel execution is enabled). It sets only MaxParallelTools; enabling
// parallelism is a separate, explicit choice — pair it with
// WithParallelToolExecution(true). It no longer silently flips that flag.
func WithMaxParallelTools(n int) Option {
	return func(r *GenerateTextRequest) { r.MaxParallelTools = n }
}

// WithPrepareStep sets a callback invoked before each tool-loop step for
// per-step configuration overrides.
func WithPrepareStep(fn PrepareStepFunc) Option {
	return func(r *GenerateTextRequest) { r.PrepareStep = fn }
}

// WithActiveTools filters the tool set to only these tool names for the run.
func WithActiveTools(names ...string) Option {
	return func(r *GenerateTextRequest) {
		r.ActiveTools = append([]string{}, names...)
	}
}

// WithOnChunk sets the callback invoked for every engine event during streaming.
func WithOnChunk(fn func(ChunkEvent)) Option {
	return func(r *GenerateTextRequest) { r.OnChunk = fn }
}

// WithOnStepEnd sets the callback invoked after each tool-loop step completes.
func WithOnStepEnd(fn func(StepEndEvent)) Option {
	return func(r *GenerateTextRequest) { r.OnStepEnd = fn }
}

// WithOnEnd sets the callback invoked when the entire run completes.
func WithOnEnd(fn func(EndEvent)) Option {
	return func(r *GenerateTextRequest) { r.OnEnd = fn }
}

// WithOnError sets the callback invoked when the run errors.
func WithOnError(fn func(error)) Option {
	return func(r *GenerateTextRequest) { r.OnError = fn }
}

// RuntimeOption configures the Runtime itself.
type RuntimeOption func(*Runtime)

// WithDefaultModel sets the model used when no WithModel option is provided per call.
func WithDefaultModel(m LanguageModel) RuntimeOption {
	return func(rt *Runtime) { rt.defaultModel = m }
}

// WithModelResolver sets a function that resolves a model ID string to a LanguageModel.
// Useful for lazy or dynamic model selection.
func WithModelResolver(fn func(string) LanguageModel) RuntimeOption {
	return func(rt *Runtime) { rt.modelResolver = fn }
}

// Runtime holds default configuration and simplifies GenerateText / StreamText calls.
type Runtime struct {
	defaultModel  LanguageModel
	modelResolver func(string) LanguageModel
}

// NewRuntime creates a Runtime with the supplied options.
func NewRuntime(opts ...RuntimeOption) *Runtime {
	rt := &Runtime{}
	for _, o := range opts {
		o(rt)
	}
	return rt
}

// resolveModel returns the effective LanguageModel for a request.
// It prefers whatever the request already has (set by WithModel), then the
// Runtime default, then panics if neither is configured.
func (rt *Runtime) resolveModel(req *GenerateTextRequest) {
	if req.Model != nil {
		return
	}
	if rt.defaultModel != nil {
		req.Model = rt.defaultModel
		return
	}
	panic("ai.Runtime: no model configured – use WithDefaultModel or pass WithModel per call")
}

// buildRequest constructs a GenerateTextRequest from a prompt and call options.
func (rt *Runtime) buildRequest(prompt string, opts []Option) GenerateTextRequest {
	req := NewRequest(nil, prompt).Build()
	for _, o := range opts {
		o(&req)
	}
	rt.resolveModel(&req)
	// Apply deferred middlewares after model resolution so they wrap the
	// resolved model and any later PrepareStep model override.
	applyDeferredMiddlewares(&req)
	return req
}

// GenerateText runs a full tool loop and returns the aggregated result.
// prompt is appended as a user message; options override individual fields.
func (rt *Runtime) GenerateText(ctx context.Context, prompt string, opts ...Option) (*GenerateTextResult, error) {
	req := rt.buildRequest(prompt, opts)
	return GenerateText(ctx, req)
}

// StreamText starts the tool loop and returns a *StreamResult for live streaming.
// prompt is appended as a user message; options override individual fields.
func (rt *Runtime) StreamText(ctx context.Context, prompt string, opts ...Option) *StreamResult {
	req := rt.buildRequest(prompt, opts)
	return StreamText(ctx, req)
}
