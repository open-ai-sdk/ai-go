package ai

import (
	"log/slog"

	"github.com/open-ai-sdk/ai-go/llm"
)

// RequestBuilder is the preferred way to construct GenerateTextRequest.
// GenerateTextRequest remains available as the explicit struct form.
//
// Builder methods return new top-level values and copy slices or maps they
// replace. Pointer fields and nested values retain normal Go ownership
// semantics: callers must not mutate shared values while a request uses them.
type RequestBuilder struct {
	request GenerateTextRequest
}

// NewRequest starts a request for model and appends prompt as a user message.
func NewRequest(model LanguageModel, prompt string) RequestBuilder {
	return RequestBuilder{request: GenerateTextRequest{
		Model:    model,
		Messages: []Message{UserMessage(prompt)},
	}}
}

// FromRequest starts a builder from explicit defaults.
func FromRequest(defaults GenerateTextRequest) RequestBuilder {
	return RequestBuilder{request: cloneGenerateTextRequest(defaults)}
}

func (b RequestBuilder) Model(model LanguageModel) RequestBuilder {
	b.request.Model = model
	return b
}

func (b RequestBuilder) Instructions(instructions string) RequestBuilder {
	b.request.Instructions = instructions
	return b
}

func (b RequestBuilder) Messages(messages ...Message) RequestBuilder {
	b.request.Messages = append([]Message(nil), messages...)
	return b
}

func (b RequestBuilder) Tools(tools *ToolSet) RequestBuilder {
	b.request.Tools = tools
	return b
}

func (b RequestBuilder) ToolChoice(choice ToolChoice) RequestBuilder {
	b.request.ToolChoice = &choice
	return b
}

func (b RequestBuilder) StopWhen(condition StopCondition) RequestBuilder {
	b.request.StopWhen = condition
	return b
}

func (b RequestBuilder) Output(output *OutputSchema) RequestBuilder {
	b.request.Output = output
	return b
}

func (b RequestBuilder) Settings(settings CallSettings) RequestBuilder {
	b.request.Settings = settings
	b.request.Settings.StopSequences = append([]string(nil), settings.StopSequences...)
	return b
}

func (b RequestBuilder) Temperature(temperature float32) RequestBuilder {
	b.request.Settings.Temperature = &temperature
	return b
}

func (b RequestBuilder) MaxTokens(maxTokens int) RequestBuilder {
	b.request.Settings.MaxTokens = maxTokens
	return b
}

func (b RequestBuilder) TopP(topP float32) RequestBuilder {
	b.request.Settings.TopP = &topP
	return b
}

func (b RequestBuilder) TopK(topK int) RequestBuilder {
	b.request.Settings.TopK = &topK
	return b
}

func (b RequestBuilder) Seed(seed int) RequestBuilder {
	b.request.Settings.Seed = &seed
	return b
}

func (b RequestBuilder) StopSequences(sequences ...string) RequestBuilder {
	b.request.Settings.StopSequences = append([]string(nil), sequences...)
	return b
}

func (b RequestBuilder) MaxSteps(maxSteps int) RequestBuilder {
	b.request.MaxSteps = maxSteps
	return b
}

func (b RequestBuilder) ProviderOptionsJSON(provider string, options map[string]any) RequestBuilder {
	b.request.ProviderOptions = withProviderOption(
		b.request.ProviderOptions,
		provider,
		cloneMap(options),
	)
	return b
}

func (b RequestBuilder) With(option llm.ProviderOption) RequestBuilder {
	if llm.IsNilProviderOption(option) {
		return b
	}
	b.request.ProviderOptions = withProviderOption(
		b.request.ProviderOptions,
		option.ProviderName(),
		option,
	)
	return b
}

func (b RequestBuilder) PrepareStep(prepare PrepareStepFunc) RequestBuilder {
	b.request.PrepareStep = prepare
	return b
}

func (b RequestBuilder) RepairToolCall(repair RepairToolCallFunc) RequestBuilder {
	b.request.RepairToolCall = repair
	return b
}

func (b RequestBuilder) ActiveTools(names ...string) RequestBuilder {
	b.request.ActiveTools = append([]string{}, names...)
	return b
}

func (b RequestBuilder) ToolsContext(context ToolsContext) RequestBuilder {
	b.request.ToolsContext = cloneMap(context)
	return b
}

func (b RequestBuilder) RuntimeContext(context RuntimeContext) RequestBuilder {
	b.request.RuntimeContext = cloneMap(context)
	return b
}

func (b RequestBuilder) ToolApproval(policies map[string]ToolApprovalFunc) RequestBuilder {
	b.request.ToolApproval = cloneMap(policies)
	return b
}

func (b RequestBuilder) ToolApprovalKey(key []byte) RequestBuilder {
	b.request.ToolApprovalKey = append([]byte(nil), key...)
	return b
}

func (b RequestBuilder) ToolApprovalReplayGuard(guard ToolApprovalReplayGuard) RequestBuilder {
	b.request.ToolApprovalReplayGuard = guard
	return b
}

func (b RequestBuilder) ToolApprovalResponder(responder ToolApprovalResponder) RequestBuilder {
	b.request.ToolApprovalResponder = responder
	return b
}

func (b RequestBuilder) OnStepEnd(callback func(StepEndEvent)) RequestBuilder {
	b.request.OnStepEnd = callback
	return b
}

func (b RequestBuilder) OnEnd(callback func(EndEvent)) RequestBuilder {
	b.request.OnEnd = callback
	return b
}

func (b RequestBuilder) OnChunk(callback func(ChunkEvent)) RequestBuilder {
	b.request.OnChunk = callback
	return b
}

func (b RequestBuilder) OnError(callback func(error)) RequestBuilder {
	b.request.OnError = callback
	return b
}

func (b RequestBuilder) SmoothStream(smooth *SmoothStream) RequestBuilder {
	b.request.SmoothStream = smooth
	return b
}

func (b RequestBuilder) Middlewares(middlewares ...LanguageModelMiddleware) RequestBuilder {
	b.request.Middlewares = append([]LanguageModelMiddleware(nil), middlewares...)
	return b
}

func (b RequestBuilder) ParallelToolExecution(enabled bool) RequestBuilder {
	b.request.ParallelToolExecution = enabled
	return b
}

func (b RequestBuilder) MaxParallelTools(maximum int) RequestBuilder {
	b.request.MaxParallelTools = maximum
	return b
}

func (b RequestBuilder) Logger(logger *slog.Logger) RequestBuilder {
	b.request.Logger = logger
	return b
}

func (b RequestBuilder) TraceContent(enabled bool) RequestBuilder {
	b.request.TraceContent = enabled
	return b
}

// Build returns an explicit request value. It copies builder-owned top-level
// slices and maps; referenced pointers and nested values remain shared.
func (b RequestBuilder) Build() GenerateTextRequest {
	return cloneGenerateTextRequest(b.request)
}
