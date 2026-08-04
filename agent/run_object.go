package agent

import (
	"context"

	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// ObjectResult contains the decoded value and the complete agent result.
type ObjectResult[T any] struct {
	Object T
	Result *Result
}

// RunObject runs r with a strict schema derived from T and decodes its final
// structured output. The derived schema replaces any output configured on r.
func RunObject[T any](ctx context.Context, r Runner) (ObjectResult[T], error) {
	schema, err := tool.StrictSchema[T]()
	if err != nil {
		return ObjectResult[T]{}, &StructuredOutputError{Kind: StructuredOutputErrorKindPrompt, Reason: "invalid output type", Cause: err}
	}
	result, err := r.Output(llm.OutputSchema{Type: "object", Schema: schema}).Run(ctx)
	output := ObjectResult[T]{Result: result}
	if err != nil {
		return output, err
	}
	if result == nil {
		return output, &StructuredOutputError{Kind: StructuredOutputErrorKindEmpty, Path: "$", Reason: "is empty"}
	}
	output.Object, err = llm.DecodeStructured[T](string(result.StructuredOutput), &llm.OutputSchema{Type: "object", Schema: schema})
	return output, err
}
