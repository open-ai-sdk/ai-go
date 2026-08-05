package agui

import (
	"encoding/json"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// Interrupt kinds and keys defined by TanStack AI's interrupt protocol. The
// binding key is namespaced by TanStack and must be spelled exactly; an
// unrecognized binding is stripped from public metadata by the client.
const (
	interruptBindingKey     = "tanstack:interruptBinding"
	interruptBindingVersion = 1
	interruptKindApproval   = "tool-approval"
	interruptReasonToolCall = "tool_call"
	interruptMetadataKind   = "approval"
)

// Client-tool interrupt vocabulary. TanStack routes a client tool through its
// pre-binding metadata path, whose predicate requires exactly these spellings:
// the reason must be one of two accepted strings, the metadata kind uses an
// underscore, and the "input" key must be present.
const (
	interruptReasonClientTool       = "tanstack:client_tool_execution"
	interruptMetadataClientToolKind = "client_tool"
)

// recordClientToolInterrupt publishes a call the UI must execute. No
// tanstack:interruptBinding is attached: a binding is honored only when it
// carries schema digests this side cannot compute without byte-parity with the
// client's canonicalizer, and one that fails validation routes to the same
// metadata-marked path as no binding at all. Omitting it is the honest form.
func (e *encoder) recordClientToolInterrupt(event aikit.StepEvent) {
	e.interrupts = append(e.interrupts, map[string]any{
		"id":         "client_tool_" + event.ToolCallID,
		"reason":     interruptReasonClientTool,
		"message":    "Client tool " + event.ToolCallName + " is ready to run",
		"toolCallId": event.ToolCallID,
		"metadata": map[string]any{
			"kind":     interruptMetadataClientToolKind,
			"toolName": event.ToolCallName,
			// Assigned unconditionally: the client tests for key presence, so
			// an omitted "input" disqualifies the interrupt entirely. A nil
			// here marshals to JSON null, which satisfies that test.
			"input": decodeToolInput(event.ToolCallArgsDelta),
		},
	})
}

// recordInterrupt captures an approval request. AG-UI has no in-band approval
// event: the run suspends and reports pending interrupts on its terminal
// RUN_FINISHED, and the client resumes by posting RunAgentInput.resume.
func (e *encoder) recordInterrupt(event aikit.StepEvent) {
	approvalID := event.ApprovalID
	if approvalID == "" {
		approvalID = "approval_" + event.ToolCallID
	}

	binding := map[string]any{
		"v":                interruptBindingVersion,
		"kind":             interruptKindApproval,
		"interruptId":      approvalID,
		"toolName":         event.ToolCallName,
		"toolCallId":       event.ToolCallID,
		"interruptedRunId": e.runID,
		"generation":       0,
	}
	// The engine's HMAC travels with the binding so a stateless resume can be
	// authenticated without server-side run state.
	if event.ApprovalSignature != "" {
		binding["signature"] = event.ApprovalSignature
	}

	input := decodeToolInput(event.ToolCallArgsDelta)
	metadata := map[string]any{
		"kind":     interruptMetadataKind,
		"toolName": event.ToolCallName,
		// Assigned unconditionally: the client tests for key presence, so an
		// approval whose arguments are empty or unparseable would otherwise be
		// treated as an interrupt this client does not own and left
		// unresolvable. A nil marshals to JSON null, which satisfies the test.
		"input":             input,
		interruptBindingKey: binding,
	}
	if input != nil {
		binding["originalArgs"] = input
	}
	if event.ApprovalIsAutomatic {
		metadata["isAutomatic"] = true
	}

	e.interrupts = append(e.interrupts, map[string]any{
		"id":         approvalID,
		"reason":     interruptReasonToolCall,
		"message":    "Approval required to run " + event.ToolCallName,
		"toolCallId": event.ToolCallID,
		"responseSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"approved": map[string]any{"type": "boolean"},
				"reason":   map[string]any{"type": "string"},
			},
			"required": []string{"approved"},
		},
		"metadata": metadata,
	})
}

func decodeToolInput(args string) any {
	if args == "" {
		return nil
	}
	var input any
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return nil
	}
	return input
}

// finishInterrupted terminates a suspended run. MESSAGES_SNAPSHOT precedes the
// terminal event because the client rebuilds its message list from it before
// rendering the approval prompt.
func (e *encoder) finishInterrupted(frames []uistream.Frame) ([]uistream.Frame, error) {
	snapshot, err := e.messagesSnapshot()
	if err != nil {
		return nil, err
	}
	frames = append(frames, snapshot...)

	state, err := e.stateSnapshot()
	if err != nil {
		return nil, err
	}
	frames = append(frames, state...)

	data := map[string]any{
		"threadId": e.threadID, "runId": e.runID,
		"outcome": map[string]any{"type": "interrupt", "interrupts": e.interrupts},
	}
	if e.totalUsage.HasValues() {
		data["usage"] = usagePayload(e.totalUsage)
	}
	last, err := e.event(eventRunFinished, data)
	return append(frames, last...), err
}

// messagesSnapshot publishes the conversation in AG-UI's message shape, which
// is not the same as the streaming event vocabulary.
//
// It carries the request's own history followed by the assistant turn built so
// far — not the assistant turn alone. MESSAGES_SNAPSHOT means the whole message
// list: its payload is typed `Message[]`, the client *replaces* its transcript
// with it, and the client's normalizer handles user, system, and tool roles.
// Publishing one message therefore deleted the user's own message and every
// earlier turn from the UI, and left the resumed request with no user text.
//
// The prior messages go out as the exact bytes that arrived. Re-encoding the
// decoded form would drop whatever this package does not model, including the
// `parts` passthrough that carries tool-call UI state.
func (e *encoder) messagesSnapshot() ([]uistream.Frame, error) {
	message := map[string]any{"id": e.messageID(), "role": "assistant"}
	if text := e.textBuf.String(); text != "" {
		message["content"] = text
	}
	if len(e.toolCalls) > 0 {
		calls := make([]map[string]any, 0, len(e.toolCalls))
		for _, record := range e.toolCalls {
			calls = append(calls, map[string]any{
				"id": record.id, "type": "function",
				"function": map[string]any{
					"name": record.name, "arguments": record.args.String(),
				},
			})
		}
		message["toolCalls"] = calls
	}
	messages := make([]any, 0, len(e.requestMessages)+1)
	for _, prior := range e.requestMessages {
		messages = append(messages, prior)
	}
	return e.event(eventMessagesSnapshot, map[string]any{
		"messages": append(messages, message),
	})
}
