// Package compattest is an external consumer of github.com/open-ai-sdk/ai-go.
//
// It imports aisdk and nothing else. That is the point: aisdk is the protocol
// layer, and a consumer that only speaks AI SDK v7 must be able to name every type
// it needs without compiling Eino, sonic, or a router. `go list -deps ./aisdk/...`
// asserts the dependency direction; this module asserts the surface is actually
// usable from outside, which a dependency check cannot show.
//
// Deliberately absent: any einoadapter symbol. The adapter's entry point arrives in
// a later phase, and asserting it here now would only pin a name that does not
// exist yet.
package compattest

import (
	"bytes"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

// fakeStreamEventer implements aisdk.StreamEventer from outside the module, proving
// the merge surface is mockable — Stream() returns the public aisdk.StepEvent, not
// an internal type an outside caller could not name.
type fakeStreamEventer struct{ ch <-chan aisdk.StepEvent }

func (f fakeStreamEventer) Stream() <-chan aisdk.StepEvent { return f.ch }
func (fakeStreamEventer) DrainUnused()                     {}

// Compile-time proof an external consumer can implement the stream surface.
var _ aisdk.StreamEventer = fakeStreamEventer{}

// scriptedEvents is the vocabulary an external producer has to be able to build:
// step boundaries, text, and a tool result with its own content shape.
func scriptedEvents() <-chan aisdk.StepEvent {
	ch := make(chan aisdk.StepEvent, 4)
	ch <- aisdk.StepEvent{Type: aisdk.StepEventStepStart}
	ch <- aisdk.StepEvent{Type: aisdk.StepEventTextDelta, TextDelta: "hi"}
	ch <- aisdk.StepEvent{
		Type: aisdk.StepEventToolResult,
		ToolResult: &aisdk.StepToolResult{
			ID: "call_1", Name: "echo", Args: `{"v":"x"}`, Output: "x",
			Content: []aisdk.ToolResultContent{
				{Type: aisdk.ToolResultContentTypeText, Text: "x"},
			},
		},
	}
	ch <- aisdk.StepEvent{Type: aisdk.StepEventStepEnd, FinishReason: aisdk.FinishReasonStop}
	close(ch)
	return ch
}

// ProduceChunks drives the scripted events through the chunk producer, which is the
// path a server takes to turn engine events into v7 UI chunks. It also checks
// FullText, since the accumulated assistant text is what a caller persists.
func ProduceChunks() ([]aisdk.Chunk, error) {
	cs := aisdk.NewChunkProducer("msg-compat-1").Produce(scriptedEvents())
	var out []aisdk.Chunk
	for c := range cs.Chunks {
		out = append(out, c)
	}
	if got := cs.FullText(); got != "hi" {
		return nil, fmt.Errorf("accumulated text: got %q, want %q", got, "hi")
	}
	return out, nil
}

// SerializeSSE proves an external caller can frame chunks as SSE without reaching
// into the module — the deleted httputil package used to own this.
func SerializeSSE() (string, error) {
	var buf bytes.Buffer
	if err := aisdk.WriteSSE(&buf, aisdk.Chunk{
		Type:   "text-delta",
		Fields: map[string]any{"id": "0", "delta": "hi"},
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// WriteViaWriter exercises the direct chunk writer, including the source shape that
// was consolidated onto one type when aitypes was promoted into aisdk.
func WriteViaWriter() (string, error) {
	var buf bytes.Buffer
	wr := aisdk.NewWriter(&buf)
	if err := wr.WriteSource(aisdk.Source{ID: "s1", URL: "https://example.com", Title: "Example"}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Compile-time proof the promoted vocabulary is nameable from outside.
var (
	_ *aisdk.APIError          = nil
	_ *aisdk.Usage             = nil
	_ aisdk.FinishReason       = aisdk.FinishReasonStop
	_ *aisdk.Adapter           = nil
	_ []aisdk.Warning          = nil
	_ aisdk.InputTokenDetails  = aisdk.InputTokenDetails{}
	_ aisdk.OutputTokenDetails = aisdk.OutputTokenDetails{}
)
