package ai_test

import (
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/ai"
)

// ExampleAPIError shows classifying a provider failure without matching on
// error text. *ai.APIError carries the parsed status code, provider error
// code/message, and request ID — never the raw response body, which is
// deliberately dropped (see the README's "Error handling" section) — and
// errors.Is maps well-known statuses to sentinels for classification.
func ExampleAPIError() {
	apiErr := ai.NewAPIError("openai", 429, nil)
	apiErr.Code = "rate_limit_exceeded"
	apiErr.RequestID = "req_abc123"
	var err error = apiErr

	var target *ai.APIError
	if errors.As(err, &target) {
		fmt.Println(target.Provider, target.StatusCode, target.RequestID)
	}
	fmt.Println(errors.Is(err, ai.ErrRateLimited))

	// Output:
	// openai 429 req_abc123
	// true
}
