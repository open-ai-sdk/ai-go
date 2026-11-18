package openrouter

import (
	"errors"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func asAPIError(err error, target **aikit.APIError) bool { return errors.As(err, target) }
