package aisdk

import (
	"encoding/json"
	"strings"
)

// UIMessage and its parts mirror ai-v7-node/packages/ai/src/ui/ui-messages.ts. This is
// what the client POSTs back on every turn, so it is the inbound half of the protocol:
// the conversation history, including tool calls, their results, and the user's
// approval decisions, all round-trip through these types.
//
// Modelled as one flat struct per part rather than a Go interface. The TS side is a
// discriminated union, and Go cannot express that in a way encoding/json understands
// without a custom unmarshaller per member. A tagged struct with Type plus pointer
// fields decodes in one pass and keeps the absent/null/zero distinction that the tool
// states depend on.

// UIMessageRole is the message author.
type UIMessageRole string

const (
	UIRoleSystem    UIMessageRole = "system"
	UIRoleUser      UIMessageRole = "user"
	UIRoleAssistant UIMessageRole = "assistant"
)

// UIMessage is one message in the client's history.
type UIMessage struct {
	ID       string          `json:"id"`
	Role     UIMessageRole   `json:"role"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Parts    []UIMessagePart `json:"parts"`
}

// UIMessagePartType identifies which part shape is populated.
//
// Tool parts are the exception to the closed set: their type is `tool-${name}`, so
// they are matched by prefix rather than compared. A dynamic tool uses the literal
// "dynamic-tool" and carries its name in ToolName instead.
type UIMessagePartType string

const (
	UIPartText           UIMessagePartType = "text"
	UIPartReasoning      UIMessagePartType = "reasoning"
	UIPartCustom         UIMessagePartType = "custom"
	UIPartSourceURL      UIMessagePartType = "source-url"
	UIPartSourceDocument UIMessagePartType = "source-document"
	UIPartFile           UIMessagePartType = "file"
	UIPartReasoningFile  UIMessagePartType = "reasoning-file"
	UIPartStepStart      UIMessagePartType = "step-start"
	UIPartDynamicTool    UIMessagePartType = "dynamic-tool"

	// UIPartToolPrefix prefixes a typed tool part: "tool-getWeather".
	UIPartToolPrefix = "tool-"
	// UIPartDataPrefix prefixes a data part: "data-plan".
	UIPartDataPrefix = "data-"
)

// UIToolState is one of the seven states a tool part can be in
// (ui-messages.ts:279-382). The set is closed, and the transitions are meaningful:
// a denial arrives as output-denied, and an approval decision the user made arrives as
// approval-responded — which is how a stateless server learns the answer.
type UIToolState string

const (
	UIToolInputStreaming    UIToolState = "input-streaming"
	UIToolInputAvailable    UIToolState = "input-available"
	UIToolApprovalRequested UIToolState = "approval-requested"
	UIToolApprovalResponded UIToolState = "approval-responded"
	UIToolOutputAvailable   UIToolState = "output-available"
	UIToolOutputError       UIToolState = "output-error"
	UIToolOutputDenied      UIToolState = "output-denied"
)

// UIToolApproval is the approval envelope on a tool part.
//
// Approved is a pointer because the three-way distinction is load-bearing: absent
// means the server asked and nobody answered yet (approval-requested), true means
// approved, false means denied. A plain bool would make "unanswered" and "denied"
// identical.
//
// Signature is the server's HMAC over (approvalId, toolCallId, toolName, hash(input)).
// Note it does not cover Approved — the client sets that itself and spreads the
// signature through unchanged. So the signature proves this server gated this tool with
// this input; it says nothing about the user's answer.
type UIToolApproval struct {
	ID          string `json:"id"`
	Approved    *bool  `json:"approved,omitempty"`
	Reason      string `json:"reason,omitempty"`
	IsAutomatic *bool  `json:"isAutomatic,omitempty"`
	Signature   string `json:"signature,omitempty"`
}

// UIMessagePart is any part of a UIMessage.
//
// Optional booleans are pointers throughout for the same reason as UIToolApproval:
// the protocol distinguishes absent from false, and Go's omitempty cannot.
type UIMessagePart struct {
	Type UIMessagePartType `json:"type"`

	// text / reasoning
	Text string `json:"text,omitempty"`

	// State is one JSON key serving two vocabularies: "streaming"/"done" on a text or
	// reasoning part, and one of the seven UIToolState values on a tool part. They must
	// share a field — two Go fields tagged `json:"state"` make encoding/json drop both
	// silently. Use ToolStateOf for the tool reading.
	State string `json:"state,omitempty"`

	// custom
	Kind string `json:"kind,omitempty"`

	// source-url / source-document
	SourceID  string `json:"sourceId,omitempty"`
	URL       string `json:"url,omitempty"`
	Title     string `json:"title,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Filename  string `json:"filename,omitempty"`

	// data-*
	ID   string          `json:"id,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`

	// tool-* / dynamic-tool
	ToolCallID       string          `json:"toolCallId,omitempty"`
	ToolName         string          `json:"toolName,omitempty"`
	Input            json.RawMessage `json:"input,omitempty"`
	RawInput         json.RawMessage `json:"rawInput,omitempty"`
	Output           json.RawMessage `json:"output,omitempty"`
	ErrorText        string          `json:"errorText,omitempty"`
	ProviderExecuted *bool           `json:"providerExecuted,omitempty"`
	Preliminary      *bool           `json:"preliminary,omitempty"`
	Approval         *UIToolApproval `json:"approval,omitempty"`

	ProviderMetadata     map[string]any `json:"providerMetadata,omitempty"`
	CallProviderMetadata map[string]any `json:"callProviderMetadata,omitempty"`
	ToolMetadata         map[string]any `json:"toolMetadata,omitempty"`
}

// IsToolPart reports whether this part is a tool invocation, typed or dynamic.
func (p *UIMessagePart) IsToolPart() bool {
	return p.Type == UIPartDynamicTool || strings.HasPrefix(string(p.Type), UIPartToolPrefix)
}

// IsDataPart reports whether this part is a data-${name} part.
func (p *UIMessagePart) IsDataPart() bool {
	return strings.HasPrefix(string(p.Type), UIPartDataPrefix)
}

// ToolStateOf returns State read as a tool state. Empty for a non-tool part, so a
// caller cannot mistake a text part's "done" for a tool state.
func (p *UIMessagePart) ToolStateOf() UIToolState {
	if !p.IsToolPart() {
		return ""
	}
	return UIToolState(p.State)
}

// ToolNameOf returns the tool name for a tool part.
//
// A typed tool part encodes the name in its type ("tool-getWeather") and may leave
// ToolName empty; a dynamic tool part carries it in ToolName. Callers should not
// reimplement this split.
func (p *UIMessagePart) ToolNameOf() string {
	if p.ToolName != "" {
		return p.ToolName
	}
	if strings.HasPrefix(string(p.Type), UIPartToolPrefix) {
		return strings.TrimPrefix(string(p.Type), UIPartToolPrefix)
	}
	return ""
}

// DataNameOf returns the name of a data-${name} part, without the prefix.
func (p *UIMessagePart) DataNameOf() string {
	if strings.HasPrefix(string(p.Type), UIPartDataPrefix) {
		return strings.TrimPrefix(string(p.Type), UIPartDataPrefix)
	}
	return ""
}

// ApprovalDecision reports the user's answer on a tool part.
//
// answered is false when there is no approval envelope or nobody has decided, which is
// the state a pending request sits in. A caller must not treat "not answered" as
// denied or as approved — that choice belongs to policy, not to parsing.
func (p *UIMessagePart) ApprovalDecision() (approved, answered bool) {
	if p.Approval == nil || p.Approval.Approved == nil {
		return false, false
	}
	return *p.Approval.Approved, true
}
