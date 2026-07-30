package einoadapter

import (
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// DrainAndClose consumes the remainder of an agent event iterator and closes every
// MessageStream it yields, returning how many it closed.
//
// This exists because `defer stream.Close()` inside the converter is not enough, and the
// gap is easy to miss. The converter can only close streams it actually received. When a
// consumer stops early — a client disconnect, an event budget, an error — the run goroutine
// keeps producing events, and each unread event may carry a MessageStream wrapping a live
// provider response body. Those are precisely the streams no defer can reach.
//
// Upstream's answer is SetAutomaticClose (adk/interface.go:462-464), which is implemented
// with runtime.SetFinalizer (schema/stream.go:277-288). That closes the stream whenever the
// GC gets to it, which under low allocation pressure may be never. It is a backstop, not a
// guarantee, and a leaked provider connection is not something to leave to the collector.
//
// The Phase 00 spike measured why this matters: with nobody calling Next(), the run ran to
// completion anyway — 6 of 6 turns — because Send() never blocks. So abandoning an iterator
// does not stop the work; it only stops anyone from cleaning up after it.
//
// Safe to call after partial consumption, and safe to call on a nil iterator.
func DrainAndClose(iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) int {
	if iter == nil {
		return 0
	}
	var closed int
	for {
		ev, ok := iter.Next()
		if !ok {
			return closed
		}
		if closeEventStream(ev) {
			closed++
		}
	}
}

// closeEventStream closes the MessageStream on one event, if it has one.
func closeEventStream(ev *adk.TypedAgentEvent[*schema.AgenticMessage]) bool {
	if ev == nil || ev.Output == nil || ev.Output.MessageOutput == nil {
		return false
	}
	sr := ev.Output.MessageOutput.MessageStream
	if sr == nil {
		return false
	}
	sr.Close()
	return true
}
