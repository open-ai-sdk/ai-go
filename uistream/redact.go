package uistream

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// RedactStreamError returns a browser-safe message while preserving an API
// error's non-sensitive HTTP status code.
func RedactStreamError(err error) string {
	if err == nil {
		return "stream error"
	}
	var apiError *aikit.APIError
	if errors.As(err, &apiError) {
		return fmt.Sprintf("provider error (status %d)", apiError.StatusCode)
	}
	return "stream error"
}

// IsRedactedStreamError reports whether message is one produced by
// RedactStreamError.
func IsRedactedStreamError(message string) bool {
	if message == "stream error" {
		return true
	}
	const prefix = "provider error (status "
	if !strings.HasPrefix(message, prefix) || !strings.HasSuffix(message, ")") {
		return false
	}
	status, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(message, prefix), ")"))
	return err == nil && status >= 100 && status <= 999
}
