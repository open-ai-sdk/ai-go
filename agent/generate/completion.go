package generate

import (
	"context"

	"github.com/open-ai-sdk/ai-go/llm"
)

// Completion is the high-level prompt and chat capability shared by agents.
// It is intentionally separate from Agent so applications may implement the
// smaller trait without also implementing streaming and tool inspection.
type Completion interface {
	Prompt(context.Context, string) (string, error)
	Chat(context.Context, string, ...Message) (string, error)
}

// CompletionBuilderProvider is the optional capability for callers that need
// detailed agent completion configuration from an interface value.
type CompletionBuilderProvider interface {
	Completion(string) AgentCompletionRequestBuilder
}

// AgentCompletionRequestBuilder configures one ToolLoopAgent completion using
// ai-go's existing GenerateText options. Send and Stream retain the agent's
// configured tools, approval policy, callbacks, and multi-step stop condition.
type AgentCompletionRequestBuilder struct {
	agent    *ToolLoopAgent
	messages []Message
	options  []Option
}

// Completion starts an agent-bound completion with prompt as the final user
// message. Unlike llm.NewCompletion, Send may execute tools and make multiple
// model calls according to the agent's stop condition.
func (a *ToolLoopAgent) Completion(prompt string) AgentCompletionRequestBuilder {
	return AgentCompletionRequestBuilder{agent: a, messages: []Message{UserMessage(prompt)}}
}

// Prompt runs an agent completion and returns its final text.
func (a *ToolLoopAgent) Prompt(ctx context.Context, prompt string) (string, error) {
	result, err := a.Completion(prompt).Send(ctx)
	if result == nil {
		return "", err
	}
	return result.Text, err
}

// Chat runs an agent completion after history and returns its final text.
func (a *ToolLoopAgent) Chat(ctx context.Context, prompt string, history ...Message) (string, error) {
	result, err := a.Completion("").Messages(history...).Message(UserMessage(prompt)).Send(ctx)
	if result == nil {
		return "", err
	}
	return result.Text, err
}

func (b AgentCompletionRequestBuilder) Instructions(value string) AgentCompletionRequestBuilder {
	return b.with(WithInstructions(value))
}

// Options applies existing per-call options to this completion. It is the
// Go-native escape hatch for controls not represented by a convenience method
// and keeps the builder forward-compatible as new options are added.
func (b AgentCompletionRequestBuilder) Options(values ...Option) AgentCompletionRequestBuilder {
	b.options = append(append([]Option(nil), b.options...), values...)
	return b
}

func (b AgentCompletionRequestBuilder) Model(value LanguageModel) AgentCompletionRequestBuilder {
	return b.with(WithModel(value))
}

// Messages replaces the conversation history. Use Message to append a turn.
func (b AgentCompletionRequestBuilder) Messages(values ...Message) AgentCompletionRequestBuilder {
	b.messages = append([]Message(nil), values...)
	return b
}

// Message appends one history message.
func (b AgentCompletionRequestBuilder) Message(value Message) AgentCompletionRequestBuilder {
	b.messages = append(append([]Message(nil), b.messages...), value)
	return b
}

func (b AgentCompletionRequestBuilder) Tools(value *ToolSet) AgentCompletionRequestBuilder {
	return b.with(WithTools(value))
}

func (b AgentCompletionRequestBuilder) ToolChoice(value ToolChoice) AgentCompletionRequestBuilder {
	return b.with(WithToolChoice(value))
}

func (b AgentCompletionRequestBuilder) Output(value *OutputSchema) AgentCompletionRequestBuilder {
	return b.with(WithOutput(value))
}

func (b AgentCompletionRequestBuilder) Temperature(value float32) AgentCompletionRequestBuilder {
	return b.with(WithTemperature(value))
}

func (b AgentCompletionRequestBuilder) MaxTokens(value int) AgentCompletionRequestBuilder {
	return b.with(WithMaxTokens(value))
}

func (b AgentCompletionRequestBuilder) TopP(value float32) AgentCompletionRequestBuilder {
	return b.with(WithTopP(value))
}

func (b AgentCompletionRequestBuilder) TopK(value int) AgentCompletionRequestBuilder {
	return b.with(WithTopK(value))
}

func (b AgentCompletionRequestBuilder) Seed(value int) AgentCompletionRequestBuilder {
	return b.with(WithSeed(value))
}

func (b AgentCompletionRequestBuilder) StopSequences(values ...string) AgentCompletionRequestBuilder {
	return b.with(WithStopSequences(values...))
}

func (b AgentCompletionRequestBuilder) MaxSteps(value int) AgentCompletionRequestBuilder {
	return b.with(WithMaxSteps(value))
}

func (b AgentCompletionRequestBuilder) StopWhen(value StopCondition) AgentCompletionRequestBuilder {
	return b.with(WithStopWhen(value))
}

func (b AgentCompletionRequestBuilder) ActiveTools(values ...string) AgentCompletionRequestBuilder {
	return b.with(WithActiveTools(values...))
}

func (b AgentCompletionRequestBuilder) ProviderOptions(value map[string]any) AgentCompletionRequestBuilder {
	return b.with(WithProviderOptions(value))
}

func (b AgentCompletionRequestBuilder) ToolsContext(value ToolsContext) AgentCompletionRequestBuilder {
	return b.with(WithToolsContext(value))
}

func (b AgentCompletionRequestBuilder) RuntimeContext(value RuntimeContext) AgentCompletionRequestBuilder {
	return b.with(WithRuntimeContext(value))
}

func (b AgentCompletionRequestBuilder) With(value llm.ProviderOption) AgentCompletionRequestBuilder {
	if llm.IsNilProviderOption(value) {
		return b
	}
	return b.with(func(request *GenerateTextRequest) {
		request.ProviderOptions = withProviderOption(request.ProviderOptions, value.ProviderName(), value)
	})
}

// Build returns the request after agent defaults and builder options merge.
func (b AgentCompletionRequestBuilder) Build() GenerateTextRequest {
	return b.agent.mergeRequest(b.callOptions())
}

// Send runs the agent's full tool loop for this completion.
func (b AgentCompletionRequestBuilder) Send(ctx context.Context) (*GenerateTextResult, error) {
	return b.agent.Generate(ctx, b.callOptions()...)
}

// Stream starts the agent's full tool loop for this completion.
func (b AgentCompletionRequestBuilder) Stream(ctx context.Context) (*StreamResult, error) {
	return b.agent.Stream(ctx, b.callOptions()...)
}

func (b AgentCompletionRequestBuilder) with(option Option) AgentCompletionRequestBuilder {
	b.options = append(append([]Option(nil), b.options...), option)
	return b
}

func (b AgentCompletionRequestBuilder) callOptions() []Option {
	options := []Option{WithMessages(b.messages...)}
	return append(options, b.options...)
}
