package einoadapter

import (
	"errors"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

// How closure is observed here, and why not the obvious way.
//
// StreamReader.Close() calls stream.closeRecv(), which closes the stream's `closed`
// channel (schema/stream.go:429-441). That signals the SENDER: stream.send checks
// `<-s.closed` and returns closed=true (:410-416). It does NOT close the items channel, so
// Recv keeps draining whatever is buffered and reports io.EOF only once the sender closes
// (:400-408).
//
// Two consequences shape these tests:
//
//   - "Recv returns io.EOF" does not test closure. It passes for an open stream with an
//     exhausted buffer and fails for a closed stream with a full one.
//   - Closure is observable only from the writer side — which is also the thing that
//     matters, since closing is what tells a provider goroutine to stop producing and
//     release its response body.
//
// So a stream the converter DRAINS must have a closed writer, or the converter blocks in
// Recv forever; and a stream whose closure we want to OBSERVE must keep a live writer. A
// single stream cannot do both, which is why the two are built separately below.

// finiteStream is drainable: the writer closes, so a converter reading it terminates.
func finiteStream(text string) *schema.StreamReader[*schema.AgenticMessage] {
	sr, sw := schema.Pipe[*schema.AgenticMessage](2)
	sw.Send(&schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlockChunk(&schema.AssistantGenText{Text: text},
				&schema.StreamingMeta{Index: 0}),
		},
	}, nil)
	sw.Close()
	return sr
}

// liveStream keeps its writer open, so the returned probe can report whether the reader was
// closed. It stands in for a stream still attached to a provider response.
func liveStream(text string) (*schema.StreamReader[*schema.AgenticMessage], func() bool) {
	sr, sw := schema.Pipe[*schema.AgenticMessage](4)
	sw.Send(&schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlockChunk(&schema.AssistantGenText{Text: text},
				&schema.StreamingMeta{Index: 0}),
		},
	}, nil)
	// Spare capacity remains, so the probe never blocks; it only reports whether the
	// reader has gone away.
	return sr, func() bool {
		return sw.Send(&schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant}, nil)
	}
}

func streamedEvent(sr *schema.StreamReader[*schema.AgenticMessage]) *adk.TypedAgentEvent[*schema.AgenticMessage] {
	return &adk.TypedAgentEvent[*schema.AgenticMessage]{
		Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
				IsStreaming: true, MessageStream: sr,
				AgenticRole: schema.AgenticRoleTypeAssistant,
			},
		},
	}
}

// The success criterion the plan names specifically: an iterator emitting 3 events with a
// converter consuming 1 must leave 0 streams open.
//
// The plan warns the first draft's version would have "passed while the leak shipped",
// because asserting Close on a stream the converter *did* receive says nothing about the
// ones it never dequeued. So the assertion is on the abandoned two, which keep live writers.
func TestDrainAndClose_ClosesStreamsTheConverterNeverDequeued(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()

	// Event 1 is the one that gets converted, so it must be drainable.
	gen.Send(streamedEvent(finiteStream("converted")))
	// Events 2 and 3 are abandoned; their writers stay live so closure is observable.
	sr2, probe2 := liveStream("abandoned-a")
	sr3, probe3 := liveStream("abandoned-b")
	gen.Send(streamedEvent(sr2))
	gen.Send(streamedEvent(sr3))
	gen.Close()

	first, ok := iter.Next()
	if !ok {
		t.Fatal("iterator was empty")
	}
	var converted int
	for range StepEventsFrom(first, NewConvState()) {
		converted++
	}
	if converted == 0 {
		t.Fatal("the first event produced no events")
	}

	// No defer inside the converter can reach the two it never dequeued.
	closed := DrainAndClose(iter)
	if closed != 2 {
		t.Errorf("DrainAndClose closed %d streams, want 2", closed)
	}
	if !probe2() {
		t.Error("abandoned stream 2 is still open after DrainAndClose")
	}
	if !probe3() {
		t.Error("abandoned stream 3 is still open after DrainAndClose")
	}
}

// Without DrainAndClose those streams stay open. This is what proves the test above
// measures something rather than a property that holds anyway.
func TestDrainAndClose_AbandonedStreamsStayOpenWithoutIt(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()

	gen.Send(streamedEvent(finiteStream("converted")))
	sr2, probe2 := liveStream("abandoned")
	gen.Send(streamedEvent(sr2))
	gen.Close()

	first, _ := iter.Next()
	for range StepEventsFrom(first, NewConvState()) {
	}

	// Deliberately no DrainAndClose.
	if probe2() {
		t.Error("the abandoned stream closed on its own; the leak this guards against " +
			"would not be detectable, so the sibling test would prove nothing")
	}

	// Clean up, so this test does not leak what it just demonstrated.
	if n := DrainAndClose(iter); n != 1 {
		t.Errorf("cleanup closed %d streams, want 1", n)
	}
	if !probe2() {
		t.Error("cleanup did not close the abandoned stream")
	}
}

func TestDrainAndClose_SafeOnNilAndExhausted(t *testing.T) {
	if n := DrainAndClose(nil); n != 0 {
		t.Errorf("nil iterator: got %d", n)
	}

	iter, gen := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	gen.Close()
	if n := DrainAndClose(iter); n != 0 {
		t.Errorf("exhausted iterator: got %d", n)
	}
}

// Events without a stream — an interrupt, an error, a non-streaming message, nil — must not
// be counted or panicked on.
func TestDrainAndClose_IgnoresEventsWithoutStreams(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	gen.Send(&adk.TypedAgentEvent[*schema.AgenticMessage]{Err: errors.New("boom")})
	gen.Send(&adk.TypedAgentEvent[*schema.AgenticMessage]{
		Action: &adk.AgentAction{Interrupted: &adk.InterruptInfo{}},
	})
	gen.Send(&adk.TypedAgentEvent[*schema.AgenticMessage]{
		Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
				Message: &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant},
			},
		},
	})
	gen.Send(nil)
	gen.Close()

	if n := DrainAndClose(iter); n != 0 {
		t.Errorf("closed %d streams, want 0 — none of these events carries one", n)
	}
}

// Breaking out mid-iteration must still close the stream. This is the disconnect path, and
// the only converter case where closure is both necessary and observable: the writer is
// still live precisely because the stream was not drained.
func TestStepEventsFrom_ClosesStreamOnEarlyBreak(t *testing.T) {
	sr, probe := liveStream("partial")

	var seen int
	for range StepEventsFrom(streamedEvent(sr), NewConvState()) {
		seen++
		break
	}
	if seen != 1 {
		t.Fatalf("consumed %d events before break, want 1", seen)
	}
	if !probe() {
		t.Error("stream left open after an early break — a provider response body would leak")
	}
}

// A drained stream still yields its content, which is the ordinary path.
func TestStepEventsFrom_DrainsAFiniteStream(t *testing.T) {
	var got []aisdk.StepEvent
	for e := range StepEventsFrom(streamedEvent(finiteStream("hello")), NewConvState()) {
		got = append(got, e)
	}
	assertTypes(t, got,
		aisdk.StepEventTextStart, aisdk.StepEventTextDelta, aisdk.StepEventTextEnd)
	if got[1].TextDelta != "hello" {
		t.Errorf("delta = %q, want hello", got[1].TextDelta)
	}
}
