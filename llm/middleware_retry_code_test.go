package llm

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestStatuslessProviderErrorRetryPolicy(t *testing.T) {
	transient := &aikit.APIError{Code: "server_error"}
	rejected := &aikit.APIError{Code: "invalid_request_error"}
	defaultPolicy := &retrier{}
	if !defaultPolicy.shouldRetry(transient) {
		t.Fatal("explicit server_error must be retryable before content")
	}
	if defaultPolicy.shouldRetry(rejected) {
		t.Fatal("invalid request must not retry")
	}
	permissive := &retrier{opts: RetryOptions{Retryable: func(error) bool { return true }}}
	if permissive.shouldRetry(rejected) {
		t.Fatal("custom classifier bypassed provider rejection")
	}
}
