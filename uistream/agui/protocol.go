package agui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

type Option func(*config)
type config struct{ runID func() string }

// WithRunID supplies deterministic run IDs, primarily for tests.
func WithRunID(fn func() string) Option { return func(c *config) { c.runID = fn } }

// Protocol returns the minimal AG-UI SSE protocol.
func Protocol(opts ...Option) uistream.Protocol {
	c := config{runID: newRunID}
	for _, o := range opts {
		o(&c)
	}
	if c.runID == nil {
		panic("agui: nil run ID function")
	}
	return uistream.Protocol{NewEncoder: func(o uistream.Options) uistream.Encoder {
		id := c.runID()
		if v, ok := o.Extra["runId"].(string); ok && v != "" {
			id = v
		}
		return &encoder{runID: id, text: map[int]bool{}}
	}, Decoder: decoder{}, Framer: framer{}}
}

type framer struct{}

func (framer) ApplyHeaders(h http.Header) { (uistream.SSEFramer{}).ApplyHeaders(h) }
func (framer) WriteFrame(w io.Writer, f uistream.Frame) error {
	return (uistream.SSEFramer{}).WriteFrame(w, f)
}

type encoder struct {
	runID string
	text  map[int]bool
}

func (e *encoder) event(typ string, data map[string]any) ([]uistream.Frame, error) {
	data["type"] = typ
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return []uistream.Frame{{Data: b}}, nil
}
func (e *encoder) Start() ([]uistream.Frame, error) {
	return e.event("RUN_STARTED", map[string]any{"runId": e.runID})
}
func (e *encoder) Encode(ev aikit.StepEvent) ([]uistream.Frame, error) {
	switch ev.Type {
	case aikit.StepEventStepStart:
		return e.event("STEP_STARTED", map[string]any{"stepName": "step"})
	case aikit.StepEventStepEnd:
		return e.event("STEP_FINISHED", map[string]any{})
	case aikit.StepEventTextDelta:
		if !e.text[ev.StepNumber] {
			e.text[ev.StepNumber] = true
			frames, err := e.event("TEXT_MESSAGE_START", map[string]any{"messageId": e.runID})
			if err != nil {
				return nil, err
			}
			next, err := e.event("TEXT_MESSAGE_CONTENT", map[string]any{"messageId": e.runID, "delta": ev.TextDelta})
			return append(frames, next...), err
		}
		return e.event("TEXT_MESSAGE_CONTENT", map[string]any{"messageId": e.runID, "delta": ev.TextDelta})
	case aikit.StepEventToolCallStart:
		return e.event("TOOL_CALL_START", map[string]any{"toolCallId": ev.ToolCallID, "toolCallName": ev.ToolCallName})
	case aikit.StepEventToolCallDelta:
		return e.event("TOOL_CALL_ARGS", map[string]any{"toolCallId": ev.ToolCallID, "delta": ev.ToolCallArgsDelta})
	case aikit.StepEventToolCallReady:
		return e.event("TOOL_CALL_END", map[string]any{"toolCallId": ev.ToolCallID})
	case aikit.StepEventToolResult:
		if ev.ToolResult == nil {
			return nil, nil
		}
		return e.event("TOOL_CALL_RESULT", map[string]any{"toolCallId": ev.ToolResult.ID, "content": ev.ToolResult.Output})
	case aikit.StepEventToolCallInvalid:
		return e.event("TOOL_CALL_END", map[string]any{"toolCallId": ev.ToolCallID})
	}
	return nil, nil
}
func (e *encoder) Finish(err error) ([]uistream.Frame, error) {
	if err != nil {
		return e.event("RUN_ERROR", map[string]any{"message": "stream error"})
	}
	var out []uistream.Frame
	for step := range e.text {
		f, _ := e.event("TEXT_MESSAGE_END", map[string]any{"messageId": e.runID, "step": step})
		out = append(out, f...)
	}
	f, er := e.event("RUN_FINISHED", map[string]any{"runId": e.runID})
	return append(out, f...), er
}

type decoder struct{}

func (decoder) Decode(r io.Reader) (uistream.Request, error) {
	in := new(struct {
		RunID    string                           `json:"runId"`
		ThreadID string                           `json:"threadId"`
		Messages []struct{ Role, Content string } `json:"messages"`
	})
	d := json.NewDecoder(r)
	if err := d.Decode(&in); err != nil {
		return uistream.Request{}, err
	}
	if in == nil {
		return uistream.Request{}, errors.New("null request")
	}
	var trailing any
	if err := d.Decode(&trailing); !errors.Is(err, io.EOF) {
		return uistream.Request{}, errors.New("multiple JSON values")
	}
	msgs := make([]aikit.Message, 0, len(in.Messages))
	for _, m := range in.Messages {
		msgs = append(msgs, aikit.Message{Role: aikit.Role(m.Role), Content: []aikit.ContentPart{{Type: aikit.ContentPartTypeText, Text: m.Content}}})
	}
	return uistream.Request{Messages: msgs, ID: in.ThreadID, Extra: map[string]any{"runId": in.RunID, "threadId": in.ThreadID}}, nil
}

func newRunID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "run"
	}
	return "run_" + hex.EncodeToString(b[:])
}
