package ainode

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// Protocol supplies the AI Node v7 encoder, decoder, and SSE framer.
func Protocol() uistream.Protocol {
	return uistream.Protocol{NewEncoder: func(o uistream.Options) uistream.Encoder { return &encoder{id: o.MessageID} }, Decoder: decoder{}, Framer: framer{}}
}

type framer struct{}

func (framer) ApplyHeaders(h http.Header) {
	(uistream.SSEFramer{}).ApplyHeaders(h)
	h.Set("x-vercel-ai-ui-message-stream", "v1")
	h.Set("x-accel-buffering", "no")
}
func (framer) WriteFrame(w io.Writer, f uistream.Frame) error {
	return (uistream.SSEFramer{}).WriteFrame(w, f)
}

type encoder struct {
	id              string
	textID          string
	n               int
	text, reasoning bool
	thought, finish string
	started         map[string]bool
	args            map[string]string
}

func (e *encoder) chunk(typ string, fields map[string]any) ([]uistream.Frame, error) {
	fields["type"] = typ
	b, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	out := []uistream.Frame{{Data: b}}
	if typ == "finish" {
		out = append(out, uistream.Frame{Data: []byte("[DONE]")})
	}
	return out, nil
}
func (e *encoder) chunks(items ...struct {
	typ string
	f   map[string]any
}) ([]uistream.Frame, error) {
	var out []uistream.Frame
	for _, x := range items {
		fs, err := e.chunk(x.typ, x.f)
		if err != nil {
			return nil, err
		}
		out = append(out, fs...)
	}
	return out, nil
}
func (e *encoder) Start() ([]uistream.Frame, error) {
	if e.id == "" {
		e.id = newMessageID()
	}
	e.started = map[string]bool{}
	e.args = map[string]string{}
	return e.chunk("start", map[string]any{"messageId": e.id})
}
func (e *encoder) blockEnd() []struct {
	typ string
	f   map[string]any
} {
	var o []struct {
		typ string
		f   map[string]any
	}
	if e.text {
		o = append(o, struct {
			typ string
			f   map[string]any
		}{"text-end", map[string]any{"id": e.textID}})
		e.text = false
	}
	if e.reasoning {
		f := map[string]any{"id": e.textID}
		if e.thought != "" {
			f["signature"] = e.thought
		}
		o = append(o, struct {
			typ string
			f   map[string]any
		}{"reasoning-end", f})
		e.reasoning = false
	}
	return o
}
func (e *encoder) next() { e.n++; e.textID = fmt.Sprintf("text_%d", e.n) }
func (e *encoder) Encode(ev aikit.StepEvent) ([]uistream.Frame, error) {
	item := func(t string, f map[string]any) struct {
		typ string
		f   map[string]any
	} {
		return struct {
			typ string
			f   map[string]any
		}{t, f}
	}
	switch ev.Type {
	case aikit.StepEventStepStart:
		e.next()
		e.text, e.reasoning = false, false
		e.thought = ""
		e.started = map[string]bool{}
		e.args = map[string]string{}
		return e.chunks(item("start-step", map[string]any{}))
	case aikit.StepEventTextDelta:
		o := e.blockEnd()
		if len(o) > 0 {
			e.next()
		}
		if !e.text {
			o = append(o, item("text-start", map[string]any{"id": e.textID}))
			e.text = true
		}
		f := map[string]any{"id": e.textID, "delta": ev.TextDelta}
		if ev.ProviderMetadata != nil {
			f["providerMetadata"] = ev.ProviderMetadata
		}
		o = append(o, item("text-delta", f))
		return e.chunks(o...)
	case aikit.StepEventReasoningDelta:
		o := e.blockEnd()
		if len(o) > 0 {
			e.next()
		}
		if !e.reasoning {
			o = append(o, item("reasoning-start", map[string]any{"id": e.textID}))
			e.reasoning = true
		}
		if ev.ThoughtSignature != "" {
			e.thought = ev.ThoughtSignature
		}
		f := map[string]any{"id": e.textID, "delta": ev.ReasoningDelta}
		if ev.ProviderMetadata != nil {
			f["providerMetadata"] = ev.ProviderMetadata
		}
		o = append(o, item("reasoning-delta", f))
		return e.chunks(o...)
	case aikit.StepEventToolCallStart:
		if ev.ToolCallID == "" {
			return nil, nil
		}
		o := e.blockEnd()
		if len(o) > 0 {
			e.next()
		}
		e.started[ev.ToolCallID] = true
		e.args[ev.ToolCallID] = ev.ToolCallArgsDelta
		o = append(o, item("tool-input-start", map[string]any{"toolCallId": ev.ToolCallID, "toolName": ev.ToolCallName}))
		if ev.ToolCallArgsDelta != "" {
			o = append(o, item("tool-input-delta", map[string]any{"toolCallId": ev.ToolCallID, "inputTextDelta": ev.ToolCallArgsDelta}))
		}
		return e.chunks(o...)
	case aikit.StepEventToolCallDelta:
		if !e.started[ev.ToolCallID] || ev.ToolCallArgsDelta == "" || json.Valid([]byte(e.args[ev.ToolCallID])) {
			return nil, nil
		}
		e.args[ev.ToolCallID] += ev.ToolCallArgsDelta
		return e.chunks(item("tool-input-delta", map[string]any{"toolCallId": ev.ToolCallID, "inputTextDelta": ev.ToolCallArgsDelta}))
	case aikit.StepEventToolCallReady:
		a := ev.ToolCallArgsDelta
		if a == "" {
			a = e.args[ev.ToolCallID]
		}
		var input any
		if json.Unmarshal([]byte(a), &input) != nil {
			input = map[string]string{"raw": a}
		}
		return e.chunks(item("tool-input-available", map[string]any{"toolCallId": ev.ToolCallID, "toolName": ev.ToolCallName, "input": input}))
	case aikit.StepEventToolResult:
		if ev.ToolResult == nil {
			return nil, nil
		}
		o := []struct {
			typ string
			f   map[string]any
		}{}
		if ev.ToolResult.ID != "" && !e.started[ev.ToolResult.ID] {
			e.started[ev.ToolResult.ID] = true
			var input any
			if json.Unmarshal([]byte(ev.ToolResult.Args), &input) != nil {
				input = map[string]string{"raw": ev.ToolResult.Args}
			}
			o = append(o, item("tool-input-available", map[string]any{"toolCallId": ev.ToolResult.ID, "toolName": ev.ToolResult.Name, "input": input}))
		}
		o = append(o, item("tool-output-available", map[string]any{"toolCallId": ev.ToolResult.ID, "output": ev.ToolResult.Output}))
		return e.chunks(o...)
	case aikit.StepEventToolApprovalRequest:
		approvalID := ev.ApprovalID
		if approvalID == "" {
			approvalID = ev.ToolCallID
		}
		f := map[string]any{"approvalId": approvalID, "toolCallId": ev.ToolCallID, "toolName": ev.ToolCallName, "args": ev.ToolCallArgsDelta}
		if ev.ApprovalIsAutomatic {
			f["isAutomatic"] = true
		}
		if ev.ApprovalSignature != "" {
			f["signature"] = ev.ApprovalSignature
		}
		return e.chunks(item("tool-approval-request", f))
	case aikit.StepEventToolOutputDenied:
		return e.chunks(item("tool-output-denied", map[string]any{"toolCallId": ev.ToolCallID}))
	case aikit.StepEventToolCallInvalid:
		return e.chunks(item("tool-input-error", map[string]any{"toolCallId": ev.ToolCallID, "toolName": ev.ToolCallName, "errorText": fmt.Sprintf("invalid JSON arguments for tool %q", ev.ToolCallName)}))
	case aikit.StepEventStepEnd:
		o := e.blockEnd()
		o = append(o, item("finish-step", map[string]any{}))
		switch ev.FinishReason {
		case aikit.FinishReasonStop:
			e.finish = "stop"
		case aikit.FinishReasonToolCalls:
			e.finish = "tool-calls"
		case aikit.FinishReasonLength:
			e.finish = "length"
		case aikit.FinishReasonContentFilter:
			e.finish = "content-filter"
		case aikit.FinishReasonError:
			e.finish = "error"
		default:
			e.finish = "other"
		}
		return e.chunks(o...)
	case aikit.StepEventDone:
		f := map[string]any{}
		if e.finish != "" {
			f["finishReason"] = e.finish
		}
		return e.chunks(item("finish", f))
	case aikit.StepEventSource:
		if ev.Source == nil || ev.Source.URL == "" {
			return nil, nil
		}
		return e.chunks(item("source-url", map[string]any{"sourceId": ev.Source.ID, "url": ev.Source.URL, "title": ev.Source.Title}))
	}
	return nil, nil
}
func (e *encoder) Finish(err error) ([]uistream.Frame, error) {
	if err == nil {
		return nil, nil
	}
	o := e.blockEnd()
	o = append(o, struct {
		typ string
		f   map[string]any
	}{"error", map[string]any{"errorText": "stream error"}}, struct {
		typ string
		f   map[string]any
	}{"finish", map[string]any{"finishReason": "error"}})
	return e.chunks(o...)
}

type decoder struct{}

func (decoder) Decode(r io.Reader) (uistream.Request, error) {
	raw := new(envelope)
	d := json.NewDecoder(r)
	if err := d.Decode(&raw); err != nil {
		return uistream.Request{}, err
	}
	if raw == nil {
		return uistream.Request{}, errors.New("null request envelope")
	}
	var x any
	if err := d.Decode(&x); !errors.Is(err, io.EOF) {
		if err == nil {
			return uistream.Request{}, errors.New("multiple JSON values")
		}
		return uistream.Request{}, err
	}
	msgs := toAIMessages(raw.Messages)
	id := ""
	if raw.Trigger == "regenerate-message" {
		id = raw.MessageID
	}
	if id == "" {
		id = newMessageID()
	}
	return uistream.Request{Messages: msgs, MessageID: id, ID: raw.ID, Body: raw.Body, Metadata: raw.Metadata, Trigger: raw.Trigger}, nil
}

type envelope struct {
	ID        string            `json:"id"`
	Messages  []envelopeMessage `json:"messages"`
	Body      map[string]any    `json:"body,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
	Trigger   string            `json:"trigger,omitempty"`
	MessageID string            `json:"messageId,omitempty"`
}
type envelopeMessage struct {
	Role    string         `json:"role"`
	Content string         `json:"content,omitempty"`
	Parts   []envelopePart `json:"parts,omitempty"`
}
type envelopeApproval struct {
	ID        string `json:"id"`
	Approved  *bool  `json:"approved,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Signature string `json:"signature,omitempty"`
}
type envelopePart struct {
	Type       string            `json:"type"`
	Text       string            `json:"text,omitempty"`
	URL        string            `json:"url,omitempty"`
	MediaType  string            `json:"mediaType,omitempty"`
	Name       string            `json:"name,omitempty"`
	FileID     string            `json:"fileId,omitempty"`
	Data       []byte            `json:"data,omitempty"`
	ToolCallID string            `json:"toolCallId,omitempty"`
	ToolName   string            `json:"toolName,omitempty"`
	Input      json.RawMessage   `json:"input,omitempty"`
	Output     any               `json:"output,omitempty"`
	ErrorText  string            `json:"errorText,omitempty"`
	State      string            `json:"state,omitempty"`
	Approval   *envelopeApproval `json:"approval,omitempty"`
}

func toAIMessages(in []envelopeMessage) []aikit.Message {
	out := make([]aikit.Message, 0, len(in))
	for _, m := range in {
		parts := toParts(m.Parts)
		if len(m.Parts) == 0 {
			parts = []aikit.ContentPart{aikit.TextPart(m.Content)}
		}
		if len(parts) > 0 {
			out = append(out, aikit.Message{Role: aikit.Role(m.Role), Content: parts})
		}
		for _, p := range m.Parts {
			if p.State == "approval-responded" && p.Approval != nil && p.Approval.Approved != nil {
				out = append(out, aikit.Message{Role: aikit.RoleUser, Content: []aikit.ContentPart{aikit.ToolApprovalResponsePart(p.Approval.ID, p.Approval.Signature, *p.Approval.Approved, p.Approval.Reason)}})
			}
		}
	}
	return out
}
func toParts(in []envelopePart) []aikit.ContentPart {
	out := make([]aikit.ContentPart, 0, len(in))
	for _, p := range in {
		switch p.Type {
		case "text":
			out = append(out, aikit.TextPart(p.Text))
		case "image", "file":
			part := aikit.ContentPart{Type: aikit.ContentPartTypeFile, MediaType: p.MediaType, Filename: p.Name}
			if p.Type == "image" && part.MediaType == "" {
				part.MediaType = "image"
			}
			if p.FileID != "" {
				part.FileID = p.FileID
			} else if len(p.Data) > 0 {
				part.Data = p.Data
			} else {
				part.FileURL = p.URL
			}
			out = append(out, part)
		default:
			if p.Type != "tool-invocation" && p.Type != "dynamic-tool" && !(len(p.Type) > 5 && p.Type[:5] == "tool-") {
				continue
			}
			name := p.ToolName
			if name == "" && len(p.Type) > 5 {
				name = p.Type[5:]
			}
			call := aikit.ContentPart{Type: aikit.ContentPartTypeToolCall, ToolCallID: p.ToolCallID, ToolCallName: name, ToolCallArgs: p.Input}
			if p.Approval != nil {
				call.ToolApprovalID = p.Approval.ID
				call.ToolApprovalSignature = p.Approval.Signature
			}
			switch p.State {
			case "call", "partial-call", "input-streaming", "input-available", "approval-requested", "approval-responded":
				out = append(out, call)
			case "result", "output-available", "output-denied", "output-error":
				out = append(out, call)
				encoded := p.ErrorText
				if p.State != "output-error" {
					if s, ok := p.Output.(string); ok {
						encoded = s
					} else if p.Output != nil {
						b, _ := json.Marshal(p.Output)
						encoded = string(b)
					}
				}
				out = append(out, aikit.ToolResultPart(p.ToolCallID, name, encoded))
			}
		}
	}
	return out
}
func newMessageID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "msg"
	}
	return "msg_" + hex.EncodeToString(b[:])
}
