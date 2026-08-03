package llm

import "github.com/open-ai-sdk/ai-go/aikit"

// RequestBuilder constructs a Request. Every method returns a new top-level
// builder value and copies slices or maps it replaces. Pointer fields and
// nested values retain normal Go ownership semantics: callers must not mutate
// shared values while a request uses them.
type RequestBuilder struct {
	request Request
}

// NewRequest starts a request with prompt appended as a user message.
func NewRequest(prompt string) RequestBuilder {
	return RequestBuilder{}.Prompt(prompt)
}

// FromRequest starts a builder from explicit defaults.
func FromRequest(defaults Request) RequestBuilder {
	return RequestBuilder{request: cloneRequest(defaults)}
}

// Instructions sets the system instructions.
func (b RequestBuilder) Instructions(instructions string) RequestBuilder {
	b.request.Instructions = instructions
	return b
}

// Messages replaces the conversation history.
func (b RequestBuilder) Messages(messages ...aikit.Message) RequestBuilder {
	b.request.Messages = append([]aikit.Message(nil), messages...)
	return b
}

// Prompt appends a user prompt to the conversation history.
func (b RequestBuilder) Prompt(prompt string) RequestBuilder {
	b.request.Messages = append(append([]aikit.Message(nil), b.request.Messages...), aikit.Message{
		Role:    aikit.RoleUser,
		Content: []aikit.ContentPart{{Type: aikit.ContentPartTypeText, Text: prompt}},
	})
	return b
}

// Tools replaces the callable tool descriptions.
func (b RequestBuilder) Tools(tools ...aikit.ToolDefinition) RequestBuilder {
	b.request.Tools = append([]aikit.ToolDefinition(nil), tools...)
	return b
}

// ToolChoice sets the tool-selection policy.
func (b RequestBuilder) ToolChoice(choice aikit.ToolChoice) RequestBuilder {
	b.request.ToolChoice = &choice
	return b
}

// Output sets the structured-output schema.
func (b RequestBuilder) Output(output OutputSchema) RequestBuilder {
	b.request.Output = &output
	return b
}

// Settings replaces common call settings.
func (b RequestBuilder) Settings(settings CallSettings) RequestBuilder {
	b.request.Settings = cloneCallSettings(settings)
	return b
}

// Temperature sets the sampling temperature.
func (b RequestBuilder) Temperature(temperature float32) RequestBuilder {
	b.request.Settings.Temperature = &temperature
	return b
}

// MaxTokens sets the maximum output-token count.
func (b RequestBuilder) MaxTokens(maxTokens int) RequestBuilder {
	b.request.Settings.MaxTokens = maxTokens
	return b
}

// TopP sets nucleus-sampling probability mass.
func (b RequestBuilder) TopP(topP float32) RequestBuilder {
	b.request.Settings.TopP = &topP
	return b
}

// TopK limits the next-token candidates.
func (b RequestBuilder) TopK(topK int) RequestBuilder {
	b.request.Settings.TopK = &topK
	return b
}

// Seed requests deterministic sampling.
func (b RequestBuilder) Seed(seed int) RequestBuilder {
	b.request.Settings.Seed = &seed
	return b
}

// StopSequences replaces the stop sequences.
func (b RequestBuilder) StopSequences(sequences ...string) RequestBuilder {
	b.request.Settings.StopSequences = append([]string(nil), sequences...)
	return b
}

// ProviderOptionsJSON sets provider options decoded from JSON. Typed callers
// should prefer [RequestBuilder.With].
func (b RequestBuilder) ProviderOptionsJSON(provider string, options map[string]any) RequestBuilder {
	b.request.ProviderOptions = withProviderOption(
		b.request.ProviderOptions,
		provider,
		cloneMap(options),
	)
	return b
}

// With attaches typed provider-specific options. A later option for the same
// provider replaces the earlier value.
func (b RequestBuilder) With(option ProviderOption) RequestBuilder {
	if IsNilProviderOption(option) {
		return b
	}
	b.request.ProviderOptions = withProviderOption(
		b.request.ProviderOptions,
		option.ProviderName(),
		option,
	)
	return b
}

// ToolsContext sets per-tool context.
func (b RequestBuilder) ToolsContext(context aikit.ToolsContext) RequestBuilder {
	b.request.ToolsContext = cloneMap(context)
	return b
}

// RuntimeContext sets run-wide tool context.
func (b RequestBuilder) RuntimeContext(context aikit.RuntimeContext) RequestBuilder {
	b.request.RuntimeContext = cloneMap(context)
	return b
}

// Build returns a request value. It copies builder-owned top-level slices and
// maps; referenced pointers and nested values remain shared.
func (b RequestBuilder) Build() Request {
	return cloneRequest(b.request)
}

func cloneRequest(request Request) Request {
	if request.Messages != nil {
		messages := make([]aikit.Message, len(request.Messages))
		for i := range request.Messages {
			messages[i] = request.Messages[i].Clone()
		}
		request.Messages = messages
	}
	request.Tools = append([]aikit.ToolDefinition(nil), request.Tools...)
	request.Settings = cloneCallSettings(request.Settings)
	request.ProviderOptions = cloneMap(request.ProviderOptions)
	request.ToolsContext = cloneMap(request.ToolsContext)
	request.RuntimeContext = cloneMap(request.RuntimeContext)
	return request
}

func cloneCallSettings(settings CallSettings) CallSettings {
	settings.StopSequences = append([]string(nil), settings.StopSequences...)
	return settings
}

func withProviderOption(options map[string]any, provider string, option any) map[string]any {
	options = cloneMap(options)
	if options == nil {
		options = make(map[string]any)
	}
	options[provider] = option
	return options
}

func cloneMap[M ~map[K]V, K comparable, V any](values M) M {
	if values == nil {
		return nil
	}
	cloned := make(M, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
