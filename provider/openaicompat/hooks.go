package openaicompat

import "github.com/open-ai-sdk/ai-go/llm"

// Compat supplies the endpoint and authentication behavior required by an
// OpenAI-compatible provider.
type Compat interface {
	BaseURL() string
	AuthHeader(apiKey string) (name, value string)
}

// Named optionally supplies the provider name used in errors and metadata.
type Named interface {
	ProviderName() string
}

// CapabilityProvider optionally declares supported Chat Completions features.
type CapabilityProvider interface {
	Capabilities() CapabilityFlags
}

// ToolSanitizer optionally adjusts function-tool schemas for provider quirks.
type ToolSanitizer interface {
	SanitizeTools(tools []map[string]any) []map[string]any
}

// RequestRewriter optionally adjusts the encoded request before it is sent.
// The request is already encoded into its JSON object form.
type RequestRewriter interface {
	RewriteRequest(req llm.Request, body map[string]any) (map[string]any, error)
}

// DecodeConfigurator optionally supplies request-local response decoding
// hooks. It is called once per stream so implementations can safely keep
// per-stream state such as source de-duplication.
type DecodeConfigurator interface {
	DecodeParams() SSEDecodeParams
}
