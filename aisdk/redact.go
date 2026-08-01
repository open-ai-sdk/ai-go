package aisdk

import (
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// redactStreamError produces the error text sent to an untrusted UI stream.
// Redaction is unconditional: a raw provider error can carry org/project/request
// identifiers and attacker-echoed prompt text, none of which belongs in a
// browser payload. Consumers who need the detail inspect the typed *aikit.APIError
// (status/code/message/request-ID) on the server instead.
//
// For a provider HTTP failure only the status code — which is not sensitive — is
// surfaced. Everything else collapses to a generic message.
func redactStreamError(err error) string {
	if err == nil {
		return "stream error"
	}
	var apiErr *aikit.APIError
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("provider error (status %d)", apiErr.StatusCode)
	}
	return "stream error"
}
