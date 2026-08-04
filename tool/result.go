package tool

import (
	"context"
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/internal/jsonclone"
)

// ExecutionResult separates ordered model-visible output from host-only
// metadata. Metadata is never included in Output or its compatibility views.
type ExecutionResult struct {
	Output   Output
	Metadata map[string]any
}

// Clone returns an independently owned execution result.
func (r ExecutionResult) Clone() ExecutionResult {
	r.Output = r.Output.Clone()
	r.Metadata = jsonclone.Map(r.Metadata)
	return r
}

// ResultInvokable is the additive rich invocation capability. Implementers
// may continue to expose Invokable only; Set adapts those tools safely.
type ResultInvokable interface {
	Definition
	InvokeResult(context.Context, json.RawMessage) (ExecutionResult, error)
}

// ResultFromLegacy adapts a released raw-byte result without guessing text:
// valid JSON remains JSON, otherwise the bytes are literal text.
func ResultFromLegacy(raw json.RawMessage) ExecutionResult {
	if json.Valid(raw) {
		output, err := JSON(raw)
		if err == nil {
			return ExecutionResult{Output: output}
		}
	}
	return ExecutionResult{Output: Text(string(raw))}
}
