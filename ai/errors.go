package ai

import (
	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

type (
	APIError                  = aikit.APIError
	CompletionError           = llm.CompletionError
	CompletionErrorKind       = llm.CompletionErrorKind
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

	StructuredOutputErrorKindPrompt        = agent.StructuredOutputErrorKindPrompt
	StructuredOutputErrorKindJSONDecode    = agent.StructuredOutputErrorKindJSONDecode
	StructuredOutputErrorKindValidation    = agent.StructuredOutputErrorKindValidation
	StructuredOutputErrorKindEmpty         = agent.StructuredOutputErrorKindEmpty
	StructuredOutputErrorKindPromptFailure = agent.StructuredOutputErrorKindPromptFailure
	StructuredOutputErrorKindEmptyResponse = agent.StructuredOutputErrorKindEmptyResponse
)

var (
	ErrRateLimited   = aikit.ErrRateLimited
	ErrContextLength = aikit.ErrContextLength
	ErrUnauthorized  = aikit.ErrUnauthorized
	ErrNoSuchTool    = tool.ErrNoSuchTool
)

func NewAPIError(provider string, statusCode int, wrapped error) *APIError {
	return aikit.NewAPIError(provider, statusCode, wrapped)
}

func RetryableStatusCode(code int) bool { return aikit.RetryableStatusCode(code) }
