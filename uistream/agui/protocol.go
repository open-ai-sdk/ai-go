package agui

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/open-ai-sdk/ai-go/uistream"
)

type (
	Option func(*config)
	config struct {
		runID           func() string
		stepEvents      bool
		structuredStart bool
	}
)

// WithRunID supplies fallback run IDs, primarily for deterministic tests.
func WithRunID(fn func() string) Option { return func(c *config) { c.runID = fn } }

// WithStepEvents emits STEP_STARTED and STEP_FINISHED for agent step
// boundaries. It is off by default because TanStack AI overloads those events
// as its reasoning transport: its stream processor turns a STEP_FINISHED into a
// thinking part even when the event carries no content, so bare step markers
// render an empty "thinking" block on every step. Enable this only for AG-UI
// clients that read step boundaries literally.
func WithStepEvents() Option { return func(c *config) { c.stepEvents = true } }

// WithStructuredOutputStart announces a structured run before its first text
// delta. TanStack accumulates assistant text into a structured-output part, and
// exposes a progressively parsed partial object, only for messages it saw
// announced; without the announcement the JSON lands in a plain text part and
// the partial never populates.
//
// It is opt-in because the encoder cannot infer it: the engine reports
// structured output only at the end of a run, long after the stream opened.
func WithStructuredOutputStart() Option {
	return func(c *config) { c.structuredStart = true }
}

// Protocol returns the AG-UI SSE protocol used by TanStack AI clients.
func Protocol(opts ...Option) uistream.Protocol {
	c := config{runID: newRunID}
	for _, option := range opts {
		option(&c)
	}
	if c.runID == nil {
		panic("agui: nil run ID function")
	}
	return uistream.Protocol{
		NewEncoder: func(options uistream.Options) uistream.Encoder {
			return newEncoder(c, options)
		},
		Decoder: decoder{},
		Framer:  framer{},
	}
}

func newEncoder(c config, options uistream.Options) *encoder {
	e := &encoder{
		runID:           c.runID(),
		emitSteps:       c.stepEvents,
		structuredStart: c.structuredStart,
		openTools:       make(map[string]bool),
		toolCallIndex:   make(map[string]*toolCallRecord),
	}
	if value, ok := options.Extra["runId"].(string); ok && value != "" {
		e.runID = value
	}
	if value, ok := options.Extra["threadId"].(string); ok {
		e.threadID = value
	}
	if value, ok := options.Extra["parentRunId"].(string); ok {
		e.parentRunID = value
	}
	// A json.RawMessage(nil) boxed in an any is a non-nil interface, so an
	// absent "state" key would otherwise echo {"snapshot":null} and a
	// conforming client would clobber its own state with it.
	if value, ok := options.Extra["state"]; ok && hasStateValue(value) {
		e.state, e.hasState = value, true
	}
	if value, ok := options.Extra[messagesExtraKey].([]json.RawMessage); ok {
		e.requestMessages = value
	}
	return e
}

// hasStateValue reports whether caller-supplied run state carries anything
// worth echoing. Raw JSON is checked for content rather than for a nil
// interface, which it never is once boxed.
func hasStateValue(value any) bool {
	if value == nil {
		return false
	}
	if raw, ok := value.(json.RawMessage); ok {
		trimmed := bytes.TrimSpace(raw)
		return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
	}
	return true
}

type framer struct{}

// AG-UI frames carry no event name: the client reads only `data:` lines and the
// event kind travels in the JSON `type` field. There is no [DONE] sentinel
// either — RUN_FINISHED terminates the stream, and TanStack's SSE parser warns
// that [DONE] is deprecated.
func (framer) ApplyHeaders(header http.Header) { (uistream.SSEFramer{}).ApplyHeaders(header) }

func (framer) WriteFrame(w io.Writer, frame uistream.Frame) error {
	return (uistream.SSEFramer{}).WriteFrame(w, frame)
}

func newRunID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "run"
	}
	return "run_" + hex.EncodeToString(value[:])
}
