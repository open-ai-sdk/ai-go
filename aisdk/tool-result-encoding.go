package aisdk

import "encoding/json"

// The wire encoding for tool results that are not a plain value.
//
// This is a contract, not a formatting choice. Eino's FunctionToolResult.Content is
// []*FunctionToolResultContentBlock restricted to text|image|audio|video|file
// (schema/agentic_message.go:305-314), so the reference's structured outputs —
// {type:'execution-denied', reason} and {type:'error-text', value} — have nowhere to live
// except inside a text block. That means the JSON shape below is what a model sees, what
// gets persisted, and what any other reader of stored history has to parse.
//
// Ad-hoc formatting here would be a silent compatibility surface. It is named, versioned
// by its own type field, and round-trip tested.

// ToolResultEncodingKind distinguishes the non-value results.
type ToolResultEncodingKind string

const (
	// ToolResultExecutionDenied is the synthetic result for a tool the user refused
	// before it ran (convert-to-model-messages.ts:320-336).
	ToolResultExecutionDenied ToolResultEncodingKind = "execution-denied"

	// ToolResultErrorText is the result for a call that reached the output-denied state,
	// meaning the denial was already carried out and is being replayed (:346-362).
	ToolResultErrorText ToolResultEncodingKind = "error-text"
)

// EncodedToolResult is the JSON written into a tool result's text block.
//
// The `aisdk` prefix on Type keeps it distinguishable from a tool that legitimately
// returns an object with a `type` field — without it, a tool returning
// {"type":"error-text","value":...} would be indistinguishable from a denial.
type EncodedToolResult struct {
	Type ToolResultEncodingKind `json:"aisdkToolResult"`
	// Reason carries the user's stated reason for a denial, when they gave one.
	Reason string `json:"reason,omitempty"`
	// Value carries the error text for error-text results.
	Value string `json:"value,omitempty"`
}

// DefaultDenialText is used when a denial carries no reason, matching the reference's
// fallback string (convert-to-model-messages.ts:355-357).
const DefaultDenialText = "Tool call execution denied."

// EncodeExecutionDenied renders the synthetic result for an approval the user refused.
func EncodeExecutionDenied(reason string) string {
	return mustEncodeToolResult(EncodedToolResult{
		Type: ToolResultExecutionDenied, Reason: reason,
	})
}

// EncodeErrorText renders the result for a call already denied on a previous turn.
func EncodeErrorText(value string) string {
	if value == "" {
		value = DefaultDenialText
	}
	return mustEncodeToolResult(EncodedToolResult{
		Type: ToolResultErrorText, Value: value,
	})
}

// DecodeToolResult parses a text block back into a structured result.
//
// ok is false for ordinary tool output, which is the common case — a tool returning plain
// text or its own JSON must not be mistaken for a denial.
func DecodeToolResult(text string) (EncodedToolResult, bool) {
	if text == "" || text[0] != '{' {
		return EncodedToolResult{}, false
	}
	var out EncodedToolResult
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return EncodedToolResult{}, false
	}
	switch out.Type {
	case ToolResultExecutionDenied, ToolResultErrorText:
		return out, true
	default:
		return EncodedToolResult{}, false
	}
}

// mustEncodeToolResult marshals a struct of plain strings, which cannot fail.
func mustEncodeToolResult(v EncodedToolResult) string {
	b, err := json.Marshal(v)
	if err != nil {
		// Unreachable: every field is a string. Falling back to the plain text keeps a
		// denial legible to the model rather than emitting an empty result.
		if v.Value != "" {
			return v.Value
		}
		return DefaultDenialText
	}
	return string(b)
}
