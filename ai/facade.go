package ai

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/agent/generate"
)

func GenerateText(ctx context.Context, req GenerateTextRequest) (*GenerateTextResult, error) {
	return generate.GenerateText(ctx, req)
}

func StreamText(ctx context.Context, req GenerateTextRequest) *StreamResult {
	return generate.StreamText(ctx, req)
}

func GenerateObject[T any](ctx context.Context, req GenerateObjectRequest) (ObjectResult[T], error) {
	return generate.GenerateObject[T](ctx, req)
}

func NewStreamResult(ch <-chan StepEvent) *StreamResult { return generate.NewStreamResult(ch) }
func NewStreamResultWithTools(ch <-chan StepEvent, tools *ToolSet) *StreamResult {
	return generate.NewStreamResultWithTools(ch, tools)
}

func NewRequest(model LanguageModel, prompt string) RequestBuilder {
	return generate.NewRequest(model, prompt)
}
func FromRequest(defaults GenerateTextRequest) RequestBuilder { return generate.FromRequest(defaults) }

func NewRuntime(opts ...RuntimeOption) *Runtime { return generate.NewRuntime(opts...) }
func NewToolLoopAgent(model LanguageModel, opts ...AgentOption) *ToolLoopAgent {
	return generate.NewToolLoopAgent(model, opts...)
}
func NewRegistry() *Registry { return generate.NewRegistry() }

func NewCostTracker() *CostTracker { return generate.NewCostTracker() }
func CalculateCost(model string, usage Usage, price ModelPrice) StepCost {
	return generate.CalculateCost(model, usage, price)
}
func GetModelPrice(modelID string) (ModelPrice, bool) { return generate.GetModelPrice(modelID) }
func SetModelPrice(modelID string, price ModelPrice)  { generate.SetModelPrice(modelID, price) }

func WithFallback(models ...LanguageModel) LanguageModel { return generate.WithFallback(models...) }
func WrapLanguageModel(model LanguageModel, middlewares ...LanguageModelMiddleware) LanguageModel {
	return generate.WrapLanguageModel(model, middlewares...)
}

func PruneMessages(messages []Message, opts PruneOptions) []Message {
	return generate.PruneMessages(messages, opts)
}

func ResponseMessagesForStep(step StepOutput, tools *ToolSet) []Message {
	return generate.ResponseMessagesForStep(step, tools)
}

func ResponseMessagesForSteps(steps []StepOutput, tools *ToolSet) []Message {
	return generate.ResponseMessagesForSteps(steps, tools)
}

func TextPart(text string) ContentPart           { return generate.TextPart(text) }
func ImageURLPart(url string) ContentPart        { return generate.ImageURLPart(url) }
func FilePart(url, mediaType string) ContentPart { return generate.FilePart(url, mediaType) }
func ImageDataPart(data []byte, mediaType string) ContentPart {
	return generate.ImageDataPart(data, mediaType)
}
func ImageFileIDPart(fileID string) ContentPart { return generate.ImageFileIDPart(fileID) }
func FileDataPart(data []byte, mediaType, filename string) ContentPart {
	return generate.FileDataPart(data, mediaType, filename)
}
func FileIDPart(fileID, mediaType string) ContentPart { return generate.FileIDPart(fileID, mediaType) }
func ReasoningPart(text string) ContentPart           { return generate.ReasoningPart(text) }
func ToolCallPart(id, name string, args json.RawMessage) ContentPart {
	return generate.ToolCallPart(id, name, args)
}

func ToolResultPart(id, name, output string) ContentPart {
	return generate.ToolResultPart(id, name, output)
}

func ToolApprovalResponsePart(id, signature string, approved bool, reason string) ContentPart {
	return generate.ToolApprovalResponsePart(id, signature, approved, reason)
}
func UserMessage(text string) Message      { return generate.UserMessage(text) }
func AssistantMessage(text string) Message { return generate.AssistantMessage(text) }
func SystemMessage(text string) Message    { return generate.SystemMessage(text) }

func IsStepCount(n int) StopCondition                  { return generate.IsStepCount(n) }
func Never() StopCondition                             { return generate.Never() }
func HasToolCall(name string) StopCondition            { return generate.HasToolCall(name) }
func OutputText() *OutputSchema                        { return generate.OutputText() }
func OutputJSONObject() *OutputSchema                  { return generate.OutputJSONObject() }
func OutputObject(schema map[string]any) *OutputSchema { return generate.OutputObject(schema) }
func OutputArray(schema map[string]any) *OutputSchema  { return generate.OutputArray(schema) }
func ToolChoiceSpecific(name string) ToolChoice        { return generate.ToolChoiceSpecific(name) }

func NewMemoryToolApprovalReplayGuard() *agent.MemoryApprovalReplayGuard {
	return generate.NewMemoryToolApprovalReplayGuard()
}

func NewSmoothStream(opts ...SmoothStreamOption) *SmoothStream {
	return generate.NewSmoothStream(opts...)
}
func WithDelayMs(ms int) SmoothStreamOption                 { return generate.WithDelayMs(ms) }
func WithWordChunking() SmoothStreamOption                  { return generate.WithWordChunking() }
func WithLineChunking() SmoothStreamOption                  { return generate.WithLineChunking() }
func WithRegexChunking(pattern string) SmoothStreamOption   { return generate.WithRegexChunking(pattern) }
func WithChunkDetector(fn ChunkDetector) SmoothStreamOption { return generate.WithChunkDetector(fn) }

func WithInstructions(value string) Option              { return generate.WithInstructions(value) }
func WithMessages(messages ...Message) Option           { return generate.WithMessages(messages...) }
func WithTools(tools *ToolSet) Option                   { return generate.WithTools(tools) }
func WithToolChoice(choice ToolChoice) Option           { return generate.WithToolChoice(choice) }
func WithMaxSteps(value int) Option                     { return generate.WithMaxSteps(value) }
func WithTemperature(value float32) Option              { return generate.WithTemperature(value) }
func WithMaxTokens(value int) Option                    { return generate.WithMaxTokens(value) }
func WithTopP(value float32) Option                     { return generate.WithTopP(value) }
func WithTopK(value int) Option                         { return generate.WithTopK(value) }
func WithSeed(value int) Option                         { return generate.WithSeed(value) }
func WithStopSequences(values ...string) Option         { return generate.WithStopSequences(values...) }
func WithOutput(output *OutputSchema) Option            { return generate.WithOutput(output) }
func WithStopWhen(stop StopCondition) Option            { return generate.WithStopWhen(stop) }
func WithProviderOptions(options map[string]any) Option { return generate.WithProviderOptions(options) }
func WithToolsContext(value ToolsContext) Option        { return generate.WithToolsContext(value) }
func WithRuntimeContext(value RuntimeContext) Option    { return generate.WithRuntimeContext(value) }
func WithToolApproval(value map[string]ToolApprovalFunc) Option {
	return generate.WithToolApproval(value)
}
func WithToolApprovalKey(key []byte) Option { return generate.WithToolApprovalKey(key) }
func WithToolApprovalReplayGuard(guard ToolApprovalReplayGuard) Option {
	return generate.WithToolApprovalReplayGuard(guard)
}
func WithModel(model LanguageModel) Option            { return generate.WithModel(model) }
func WithSmoothStream(stream *SmoothStream) Option    { return generate.WithSmoothStream(stream) }
func WithRepairToolCall(fn RepairToolCallFunc) Option { return generate.WithRepairToolCall(fn) }
func WithParallelToolExecution(enabled bool) Option {
	return generate.WithParallelToolExecution(enabled)
}
func WithMaxParallelTools(value int) Option      { return generate.WithMaxParallelTools(value) }
func WithPrepareStep(fn PrepareStepFunc) Option  { return generate.WithPrepareStep(fn) }
func WithActiveTools(names ...string) Option     { return generate.WithActiveTools(names...) }
func WithOnChunk(fn func(ChunkEvent)) Option     { return generate.WithOnChunk(fn) }
func WithOnStepEnd(fn func(StepEndEvent)) Option { return generate.WithOnStepEnd(fn) }
func WithOnEnd(fn func(EndEvent)) Option         { return generate.WithOnEnd(fn) }
func WithOnError(fn func(error)) Option          { return generate.WithOnError(fn) }
func WithLogger(logger *slog.Logger) Option      { return generate.WithLogger(logger) }
func WithTracer(tracer agent.Tracer) Option      { return generate.WithTracer(tracer) }
func WithTraceContent(enabled bool) Option       { return generate.WithTraceContent(enabled) }
func WithMiddleware(values ...LanguageModelMiddleware) Option {
	return generate.WithMiddleware(values...)
}
func WithMaxRetries(value int) Option     { return generate.WithMaxRetries(value) }
func WithRetry(config RetryConfig) Option { return generate.WithRetry(config) }

func WithDefaultModel(model LanguageModel) RuntimeOption { return generate.WithDefaultModel(model) }
func WithModelResolver(fn func(string) LanguageModel) RuntimeOption {
	return generate.WithModelResolver(fn)
}

func WithAgentID(value string) AgentOption              { return generate.WithAgentID(value) }
func WithAgentTools(value *ToolSet) AgentOption         { return generate.WithAgentTools(value) }
func WithAgentInstructions(value string) AgentOption    { return generate.WithAgentInstructions(value) }
func WithAgentToolChoice(value ToolChoice) AgentOption  { return generate.WithAgentToolChoice(value) }
func WithAgentStopWhen(value StopCondition) AgentOption { return generate.WithAgentStopWhen(value) }
func WithAgentPrepareStep(value PrepareStepFunc) AgentOption {
	return generate.WithAgentPrepareStep(value)
}
func WithAgentOutput(value *OutputSchema) AgentOption { return generate.WithAgentOutput(value) }
func WithAgentProviderOptions(value map[string]any) AgentOption {
	return generate.WithAgentProviderOptions(value)
}

func WithAgentRepairToolCall(value RepairToolCallFunc) AgentOption {
	return generate.WithAgentRepairToolCall(value)
}

func WithAgentToolApproval(value map[string]ToolApprovalFunc) AgentOption {
	return generate.WithAgentToolApproval(value)
}
func WithAgentToolApprovalKey(key []byte) AgentOption { return generate.WithAgentToolApprovalKey(key) }
func WithAgentToolApprovalReplayGuard(guard ToolApprovalReplayGuard) AgentOption {
	return generate.WithAgentToolApprovalReplayGuard(guard)
}

func WithAgentToolApprovalResponder(value ToolApprovalResponder) AgentOption {
	return generate.WithAgentToolApprovalResponder(value)
}

func WithAgentParallelToolExecution(enabled bool) AgentOption {
	return generate.WithAgentParallelToolExecution(enabled)
}
func WithAgentOnStepEnd(fn func(StepEndEvent)) AgentOption { return generate.WithAgentOnStepEnd(fn) }
func WithAgentOnEnd(fn func(EndEvent)) AgentOption         { return generate.WithAgentOnEnd(fn) }
func WithAgentOnChunk(fn func(ChunkEvent)) AgentOption     { return generate.WithAgentOnChunk(fn) }
func WithAgentOnError(fn func(error)) AgentOption          { return generate.WithAgentOnError(fn) }
