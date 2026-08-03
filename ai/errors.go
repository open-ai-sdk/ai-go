package ai

import (
	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/agent/generate"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// APIError is a provider HTTP failure surfaced as a typed value; the raw
// response body is never embedded. See aikit.APIError.
type (
	APIError                  = aikit.APIError
	CompletionError           = llm.CompletionError
	CompletionErrorKind       = llm.CompletionErrorKind
	PromptError               = generate.PromptError
	PromptErrorKind           = generate.PromptErrorKind
	StructuredOutputError     = agent.StructuredOutputError
	StructuredOutputErrorKind = agent.StructuredOutputErrorKind
)

const (
	CompletionErrorKindTransport       = llm.CompletionErrorKindTransport
	CompletionErrorKindJSONDecode      = llm.CompletionErrorKindJSONDecode
	CompletionErrorKindRequest         = llm.CompletionErrorKindRequest
	CompletionErrorKindResponse        = llm.CompletionErrorKindResponse
	CompletionErrorKindProvider        = llm.CompletionErrorKindProvider
	CompletionErrorKindInvalidRequest  = llm.CompletionErrorKindInvalidRequest
	CompletionErrorKindInvalidResponse = llm.CompletionErrorKindInvalidResponse

	PromptErrorKindCompletion     = generate.PromptErrorKindCompletion
	PromptErrorKindMaxTurns       = generate.PromptErrorKindMaxTurns
	PromptErrorKindUnknownTool    = generate.PromptErrorKindUnknownTool
	PromptErrorKindDisallowedTool = generate.PromptErrorKindDisallowedTool
	PromptErrorKindToolExecution  = generate.PromptErrorKindToolExecution
	PromptErrorKindCancellation   = generate.PromptErrorKindCancellation
	PromptErrorKindMemory         = generate.PromptErrorKindMemory
	PromptErrorKindMemoryHistory  = generate.PromptErrorKindMemoryHistory

	StructuredOutputErrorKindPrompt        = agent.StructuredOutputErrorKindPrompt
	StructuredOutputErrorKindJSONDecode    = agent.StructuredOutputErrorKindJSONDecode
	StructuredOutputErrorKindValidation    = agent.StructuredOutputErrorKindValidation
	StructuredOutputErrorKindEmpty         = agent.StructuredOutputErrorKindEmpty
	StructuredOutputErrorKindPromptFailure = agent.StructuredOutputErrorKindPromptFailure
	StructuredOutputErrorKindEmptyResponse = agent.StructuredOutputErrorKindEmptyResponse
)

// Sentinel errors for errors.Is classification. *APIError.Is maps status codes
// to these, so consumers write errors.Is(err, ai.ErrRateLimited).
var (
	ErrRateLimited   = aikit.ErrRateLimited
	ErrContextLength = aikit.ErrContextLength
	ErrUnauthorized  = aikit.ErrUnauthorized
	ErrNoSuchTool    = tool.ErrNoSuchTool
)

// NewAPIError builds a typed provider error. Providers usually call
// transport.APIErrorFromResponse instead, which also parses headers and body.
func NewAPIError(provider string, statusCode int, wrapped error) *APIError {
	return aikit.NewAPIError(provider, statusCode, wrapped)
}

// RetryableStatusCode reports whether an HTTP status code is transient and worth
// retrying. Exported for provider implementations.
func RetryableStatusCode(code int) bool { return aikit.RetryableStatusCode(code) }
