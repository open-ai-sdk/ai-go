package ai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// CompletionObjectResult is the decoded value and provider response from one
// direct structured completion.
type CompletionObjectResult[T any] = llm.CompletionObjectResult[T]

// CompleteObject performs exactly one model call and decodes its text as T.
// It never enters the Agent runtime or executes tools.
func CompleteObject[T any](
	ctx context.Context,
	model LanguageModel,
	request CompletionRequest,
) (CompletionObjectResult[T], error) {
	schema, err := tool.Schema[T]()
	if err != nil {
		return CompletionObjectResult[T]{}, &agent.StructuredOutputError{
			Kind: agent.StructuredOutputErrorKindPrompt, Reason: "invalid output request", Cause: err,
		}
	}
	request.Output = &llm.OutputSchema{Type: "object", Schema: schema}
	response, err := llm.Complete(ctx, model, request)
	result := CompletionObjectResult[T]{Response: response}
	if err != nil {
		return result, &agent.StructuredOutputError{
			Kind: agent.StructuredOutputErrorKindPrompt, Reason: "completion failed", Cause: err,
		}
	}
	if response == nil || strings.TrimSpace(response.Text) == "" {
		return result, &agent.StructuredOutputError{
			Kind: agent.StructuredOutputErrorKindEmpty, Path: "$", Reason: "is empty",
		}
	}
	if err := json.Unmarshal([]byte(response.Text), &result.Object); err != nil {
		return result, &agent.StructuredOutputError{
			Kind: agent.StructuredOutputErrorKindJSONDecode, Path: "$",
			Reason: "is invalid JSON: " + err.Error(), Cause: err,
		}
	}
	return result, nil
}
