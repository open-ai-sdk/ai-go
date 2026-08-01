package generate

import (
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

type APIError = aikit.APIError

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
