package aisdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// ChatRequest is the body `useChat` POSTs.
//
// Shape taken from ui/http-chat-transport.ts:175-184, whose default body is exactly
// `{...resolvedBody, ...options.body, id, messages, trigger, messageId}`.
//
// Two fields are deliberately absent or narrowed, both settled by reading the client:
//
//   - There is no Metadata field. `requestMetadata` reaches only
//     prepareSendMessagesRequest (:165) and never enters the default body, so a server
//     that read it would be reading something the client never sends.
//   - MessageID is a string, not *string. JSON.stringify drops undefined keys, so the
//     field is omitted rather than null when unset, and a pointer would only add a nil
//     case that cannot occur.
type ChatRequest struct {
	// ID is the chat id, stable across turns.
	ID string `json:"id"`

	// Messages is the full conversation history, re-sent on every turn. This is where
	// tool results and approval decisions live — the protocol has no side channel.
	Messages []UIMessage `json:"messages"`

	// Trigger says why the client sent this request.
	Trigger ChatTrigger `json:"trigger"`

	// MessageID identifies the message being regenerated or edited. Never echo it as the
	// response id — see ResolveResponseMessageID.
	MessageID string `json:"messageId,omitempty"`

	// Body carries application fields a caller added via `body`. Kept raw because ai-go
	// has no opinion about them.
	Body map[string]json.RawMessage `json:"-"`
}

// ChatTrigger enumerates the client's reasons for a request.
type ChatTrigger string

const (
	TriggerSubmitMessage ChatTrigger = "submit-message"
	TriggerRegenerate    ChatTrigger = "regenerate-message"
	// TriggerResumeStream is the reconnect path (ui/chat.ts:616). ai-go does not
	// implement resumable streams — that needs a stream store — so it is rejected
	// explicitly rather than silently treated as a new submission, which would re-run
	// the model and bill for it twice.
	TriggerResumeStream ChatTrigger = "resume-stream"
)

// ResolveResponseMessageID returns the id the assistant message should carry.
//
// It echoes the last message's id ONLY when that message is assistant-role — a
// continuation of an existing assistant turn. Otherwise it returns "" and the caller
// generates a fresh id.
//
// Deliberately ignores req.MessageID. On the edit-and-resend path `messageId` is a USER
// message's id (ui/chat.ts:384-420), and echoing it makes the client's replaceLastMessage
// overwrite the user's own prompt (:704-713) — the user watches their question turn into
// the answer.
func (r *ChatRequest) ResolveResponseMessageID() string {
	if len(r.Messages) == 0 {
		return ""
	}
	last := r.Messages[len(r.Messages)-1]
	if last.Role != UIRoleAssistant {
		return ""
	}
	return last.ID
}

// LastUserMessage returns the most recent user-role message, or nil.
func (r *ChatRequest) LastUserMessage() *UIMessage {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == UIRoleUser {
			return &r.Messages[i]
		}
	}
	return nil
}

// DecodeLimits bounds what a request body may contain.
//
// These are not tidiness. Phase 05's validation order is fixed — signature, then schema,
// then policy — which makes HashCanonical the FIRST code to touch unvalidated browser
// JSON, and canonical serialization is recursive. encoding/json enforces no depth limit
// of its own, so without a bound here a nested body reaches that recursion before
// anything has authenticated it.
type DecodeLimits struct {
	// MaxBodyBytes caps the raw request body. 1 MiB fits a long conversation with tool
	// inputs; a body larger than this is a mistake or an attack, not a chat.
	MaxBodyBytes int64
	// MaxJSONDepth caps nesting. Matches CanonicalJSONMaxDepth, since exceeding it there
	// would fail anyway — better to reject at the edge with a clear error.
	MaxJSONDepth int
	// MaxMessages caps history length.
	MaxMessages int
	// MaxPartsPerMessage caps parts within one message.
	MaxPartsPerMessage int
	// MaxToolInputBytes caps a single tool input, which is passed to code that executes.
	MaxToolInputBytes int
}

// DefaultDecodeLimits are the documented defaults. Every value is concrete on purpose:
// "add limits with a sensible default" is how a limit ends up unset in production.
func DefaultDecodeLimits() DecodeLimits {
	return DecodeLimits{
		MaxBodyBytes:       1 << 20, // 1 MiB
		MaxJSONDepth:       CanonicalJSONMaxDepth,
		MaxMessages:        1000,
		MaxPartsPerMessage: 1000,
		MaxToolInputBytes:  128 << 10, // 128 KiB
	}
}

// DecodeChatRequest reads and validates a request body.
//
// Depth is checked with a streaming token loop rather than by unmarshalling into `any`
// and walking the result. The difference matters: unmarshalling allocates the whole
// nested structure first, so a depth check afterwards has already paid the cost it was
// meant to avoid.
func DecodeChatRequest(r io.Reader, limits DecodeLimits) (*ChatRequest, error) {
	if limits.MaxBodyBytes <= 0 {
		limits = DefaultDecodeLimits()
	}

	// One byte over the cap distinguishes "exactly at the limit" from "truncated".
	body, err := io.ReadAll(io.LimitReader(r, limits.MaxBodyBytes+1))
	if err != nil {
		return nil, &ChatRequestError{Reason: "reading body", Err: err}
	}
	if int64(len(body)) > limits.MaxBodyBytes {
		return nil, &ChatRequestError{
			Reason: fmt.Sprintf("body exceeds %d bytes", limits.MaxBodyBytes),
		}
	}

	if err := checkJSONDepth(body, limits.MaxJSONDepth); err != nil {
		return nil, err
	}

	var req ChatRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&req); err != nil {
		return nil, &ChatRequestError{Reason: "malformed JSON", Err: err}
	}

	// Application fields are kept separately so a caller can read its own `body` keys
	// without ai-go having to know them.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err == nil {
		for _, known := range []string{"id", "messages", "trigger", "messageId"} {
			delete(raw, known)
		}
		if len(raw) > 0 {
			req.Body = raw
		}
	}

	if err := req.checkShape(limits); err != nil {
		return nil, err
	}
	return &req, nil
}

// checkShape enforces the structural limits and the trigger vocabulary.
func (r *ChatRequest) checkShape(limits DecodeLimits) error {
	switch r.Trigger {
	case TriggerSubmitMessage, TriggerRegenerate:
	case TriggerResumeStream:
		return &ChatRequestError{
			Reason: "trigger \"resume-stream\" requires resumable streams, which this " +
				"server does not implement",
		}
	case "":
		// Older clients and hand-rolled callers omit it; treat as a submission rather
		// than rejecting a request that is otherwise well-formed.
		r.Trigger = TriggerSubmitMessage
	default:
		return &ChatRequestError{Reason: fmt.Sprintf("unknown trigger %q", r.Trigger)}
	}

	if len(r.Messages) > limits.MaxMessages {
		return &ChatRequestError{
			Reason: fmt.Sprintf("%d messages exceeds the limit of %d",
				len(r.Messages), limits.MaxMessages),
		}
	}
	for i, m := range r.Messages {
		if len(m.Parts) > limits.MaxPartsPerMessage {
			return &ChatRequestError{
				MessageIndex: i,
				Reason: fmt.Sprintf("%d parts exceeds the limit of %d",
					len(m.Parts), limits.MaxPartsPerMessage),
			}
		}
		for j, p := range m.Parts {
			if len(p.Input) > limits.MaxToolInputBytes {
				return &ChatRequestError{
					MessageIndex: i, PartIndex: j,
					Reason: fmt.Sprintf("tool input of %d bytes exceeds the limit of %d",
						len(p.Input), limits.MaxToolInputBytes),
				}
			}
		}
	}
	return nil
}

// checkJSONDepth walks the token stream counting nesting, without building the value.
func checkJSONDepth(body []byte, maxDepth int) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	var depth int
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return &ChatRequestError{Reason: "malformed JSON", Err: err}
		}
		switch d := tok.(type) {
		case json.Delim:
			switch d {
			case '{', '[':
				depth++
				if depth > maxDepth {
					return &ChatRequestError{
						Reason: fmt.Sprintf("JSON nesting exceeds the %d-level limit", maxDepth),
					}
				}
			case '}', ']':
				depth--
			}
		}
	}
}
