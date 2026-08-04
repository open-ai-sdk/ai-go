package agent

import (
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/llm"
)

// StructuredOutputErrorKind identifies which structured-output boundary failed.
type StructuredOutputErrorKind = llm.StructuredOutputErrorKind

const (
	StructuredOutputErrorKindPrompt        = llm.StructuredOutputErrorKindPrompt
	StructuredOutputErrorKindJSONDecode    = llm.StructuredOutputErrorKindJSONDecode
	StructuredOutputErrorKindValidation    = llm.StructuredOutputErrorKindValidation
	StructuredOutputErrorKindEmpty         = llm.StructuredOutputErrorKindEmpty
	StructuredOutputErrorKindPromptFailure = llm.StructuredOutputErrorKindPromptFailure
	StructuredOutputErrorKindEmptyResponse = llm.StructuredOutputErrorKindEmptyResponse
)

// StructuredOutputError is re-exported for source compatibility.
type StructuredOutputError = llm.StructuredOutputError

func validateStructuredOutput(raw json.RawMessage, output *OutputSchema) error {
	return llm.ValidateStructuredOutput(raw, output)
}

func validateOutputConfiguration(output *OutputSchema) error {
	return llm.ValidateOutputConfiguration(output)
}
