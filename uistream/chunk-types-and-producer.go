package uistream

import (
	"encoding/json"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aitypes"
	"github.com/open-ai-sdk/ai-go/internal/safego"
)

// Chunk is a typed UI stream chunk that can be serialized to different transports.
type Chunk struct {
	Type   string         // chunk type constant (ChunkTextDelta, etc.)
	Fields map[string]any // chunk-specific payload fields
}

// ChunkStream is a stream of typed chunks produced from engine events.
// Drain Chunks before calling FullText.
type ChunkStream struct {
	// Chunks is the output channel; it is closed when the producer is done.
	Chunks <-chan Chunk

	done chan struct{}
	text string
}

// FullText blocks until the Chunks channel is fully drained and returns the
// accumulated assistant text.
func (cs *ChunkStream) FullText() string {
	<-cs.done
	return cs.text
}

// ChunkProducer translates aitypes.StepEvents into a channel of typed Chunks.
// It holds the same per-stream state as the former Adapter internals.
type ChunkProducer struct {
	msgID string

	// per-step state — reset on each StepStart
	textBlockID      string
	textBlockCount   int
	textStarted      bool
	reasoningStarted bool
	toolInputStarted map[string]bool
	toolInputReady   map[string]bool
	toolArgsAccum    map[string]string

	// lastFinishReason stores the finish reason from the most recent StepEventStepEnd.
	lastFinishReason string
	// lastThoughtSignature stores the most recent thought signature from a reasoning delta.
	lastThoughtSignature string
}

// NewChunkProducer creates a ChunkProducer with the given message ID.
func NewChunkProducer(msgID string) *ChunkProducer {
	return &ChunkProducer{
		msgID:            msgID,
		toolInputStarted: make(map[string]bool),
		toolInputReady:   make(map[string]bool),
		toolArgsAccum:    make(map[string]string),
	}
}

// Produce starts consuming events from ch, emitting Chunks to the returned
// ChunkStream. The returned ChunkStream.Chunks channel is closed when ch is
// exhausted or an error event is received.
//
// Produce is designed for single use per ChunkProducer instance.
func (cp *ChunkProducer) Produce(ch <-chan aitypes.StepEvent) *ChunkStream {
	out := make(chan Chunk, 64)
	cs := &ChunkStream{
		Chunks: out,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(out)
		defer close(cs.done)
		defer safego.Recover(nil, recoverToChunk(out))

		// Emit the stream-start chunk first.
		out <- Chunk{Type: ChunkStart, Fields: map[string]any{"messageId": cp.msgID}}

		for ev := range ch {
			if ev.Type == aitypes.StepEventError {
				for _, c := range cp.chunksError(ev.Error) {
					out <- c
				}
				return
			}
			chunks, delta := cp.translateEvent(ev)
			for _, c := range chunks {
				out <- c
			}
			cs.text += delta
		}
	}()

	return cs
}

// translateEvent converts a single StepEvent into zero or more Chunks plus any
// text delta accumulated.
func (cp *ChunkProducer) translateEvent(ev aitypes.StepEvent) ([]Chunk, string) {
	switch ev.Type {
	case aitypes.StepEventStepStart:
		return cp.chunksStepStart(), ""
	case aitypes.StepEventTextDelta:
		return cp.chunksTextDelta(ev)
	case aitypes.StepEventReasoningDelta:
		return cp.chunksReasoningDelta(ev), ""
	case aitypes.StepEventToolCallStart:
		return cp.chunksToolCallStart(ev), ""
	case aitypes.StepEventToolCallDelta:
		return cp.chunksToolCallDelta(ev), ""
	case aitypes.StepEventToolCallReady:
		return cp.chunksToolCallReady(ev), ""
	case aitypes.StepEventToolResult:
		return cp.chunksToolResult(ev), ""
	case aitypes.StepEventToolApprovalRequest:
		fields := map[string]any{
			"approvalId": ev.ToolCallID,
			"toolCallId": ev.ToolCallID,
			"toolName":   ev.ToolCallName,
			"args":       ev.ToolCallArgsDelta,
		}
		// isAutomatic and signature are optional in the protocol; include them
		// only when set so the chunk matches node's omit-when-absent shape.
		if ev.ApprovalIsAutomatic {
			fields["isAutomatic"] = true
		}
		if ev.ApprovalSignature != "" {
			fields["signature"] = ev.ApprovalSignature
		}
		return []Chunk{{Type: ChunkToolApprovalRequest, Fields: fields}}, ""
	case aitypes.StepEventToolOutputDenied:
		return []Chunk{{Type: ChunkToolOutputDenied, Fields: map[string]any{"toolCallId": ev.ToolCallID}}}, ""
	case aitypes.StepEventToolCallInvalid:
		return cp.chunksToolCallInvalid(ev), ""
	case aitypes.StepEventSource:
		return cp.chunksSource(ev), ""
	case aitypes.StepEventStepEnd:
		if ev.FinishReason == "" {
			cp.lastFinishReason = ""
			return cp.chunksStepEnd(), ""
		}
		finishReason, ok := wireFinishReason(ev.FinishReason)
		if !ok {
			finishReason = "other"
		}
		cp.lastFinishReason = finishReason
		return cp.chunksStepEnd(), ""
	case aitypes.StepEventDone:
		fields := map[string]any{}
		if cp.lastFinishReason != "" {
			fields["finishReason"] = cp.lastFinishReason
		}
		return []Chunk{
			{Type: ChunkFinish, Fields: fields},
		}, ""
	}
	return nil, ""
}

// advanceBlockID rotates the active text/reasoning block id. Every block
// boundary (text↔reasoning, text→tool, reasoning→tool, step→step) calls this
// before the next start chunk so each conceptual block ends up with a unique
// id — mirrors ai-sdk-node's use of Anthropic's `content_block_index`, where
// the provider naturally assigns a fresh index per content block. Without
// this, downstream UIs that key parts by id (find-or-merge by `id`) collapse
// consecutive same-type blocks (e.g. text → tool → text) into one part.
func (cp *ChunkProducer) advanceBlockID() {
	cp.textBlockCount++
	cp.textBlockID = blockID(cp.textBlockCount)
}

func (cp *ChunkProducer) chunksStepStart() []Chunk {
	cp.advanceBlockID()
	cp.textStarted = false
	cp.reasoningStarted = false
	cp.lastThoughtSignature = ""
	cp.toolInputStarted = make(map[string]bool)
	cp.toolArgsAccum = make(map[string]string)
	return []Chunk{{Type: ChunkStartStep, Fields: nil}}
}

func (cp *ChunkProducer) chunksTextDelta(ev aitypes.StepEvent) ([]Chunk, string) {
	var out []Chunk
	// End active reasoning block before text starts (matches Vercel AI SDK behavior).
	if cp.reasoningStarted {
		reasoningEndFields := map[string]any{"id": cp.textBlockID}
		if cp.lastThoughtSignature != "" {
			reasoningEndFields["signature"] = cp.lastThoughtSignature
		}
		out = append(out, Chunk{Type: ChunkReasoningEnd, Fields: reasoningEndFields})
		cp.reasoningStarted = false
		// Advance so the new text block does not reuse the just-closed
		// reasoning block's id (downstream UIs key parts by id).
		cp.advanceBlockID()
	}
	if !cp.textStarted {
		out = append(out, Chunk{Type: ChunkTextStart, Fields: map[string]any{"id": cp.textBlockID}})
		cp.textStarted = true
	}
	fields := map[string]any{
		"id":    cp.textBlockID,
		"delta": ev.TextDelta,
	}
	out = append(out, Chunk{Type: ChunkTextDelta, Fields: withProviderMetadata(fields, ev.ProviderMetadata)})
	return out, ev.TextDelta
}

func (cp *ChunkProducer) chunksReasoningDelta(ev aitypes.StepEvent) []Chunk {
	var out []Chunk
	// End active text block before reasoning starts. Symmetric to chunksTextDelta's
	// reasoning-end emission — preserves chronological order when a model
	// interleaves text and reasoning within a step.
	if cp.textStarted {
		out = append(out, Chunk{Type: ChunkTextEnd, Fields: map[string]any{"id": cp.textBlockID}})
		cp.textStarted = false
		cp.advanceBlockID()
	}
	if !cp.reasoningStarted {
		out = append(out, Chunk{Type: ChunkReasoningStart, Fields: map[string]any{"id": cp.textBlockID}})
		cp.reasoningStarted = true
	}
	if ev.ThoughtSignature != "" {
		cp.lastThoughtSignature = ev.ThoughtSignature
	}
	fields := map[string]any{
		"id":    cp.textBlockID,
		"delta": ev.ReasoningDelta,
	}
	out = append(out, Chunk{Type: ChunkReasoningDelta, Fields: withProviderMetadata(fields, ev.ProviderMetadata)})
	return out
}

func (cp *ChunkProducer) chunksToolCallStart(ev aitypes.StepEvent) []Chunk {
	tcID := ev.ToolCallID
	if tcID == "" {
		return nil
	}
	cp.toolInputStarted[tcID] = true
	cp.toolArgsAccum[tcID] = ev.ToolCallArgsDelta

	var out []Chunk
	// End any active text block before the tool call starts. Without this,
	// a model that interleaves text → tool → text within a single step keeps
	// the same text block id for all text deltas, so downstream consumers
	// concatenate every text segment into the FIRST text part and render
	// tool calls after it — losing chronological order. Advancing the block
	// id ensures the text after the tool gets a fresh text-start with a new
	// id and lands as a separate part.
	if cp.textStarted {
		out = append(out, Chunk{Type: ChunkTextEnd, Fields: map[string]any{"id": cp.textBlockID}})
		cp.textStarted = false
		cp.advanceBlockID()
	}
	// End any active reasoning block before the tool call starts so the
	// downstream PersistedMessageBuilder appends the reasoning part BEFORE
	// the tool part — preserves chronological order on rehydration.
	// Mirrors chunksTextDelta's reasoning-end emission at the text/reasoning
	// boundary (matches ai-sdk-node's per-block reasoning-start/-end events).
	if cp.reasoningStarted {
		reasoningEndFields := map[string]any{"id": cp.textBlockID}
		if cp.lastThoughtSignature != "" {
			reasoningEndFields["signature"] = cp.lastThoughtSignature
		}
		out = append(out, Chunk{Type: ChunkReasoningEnd, Fields: reasoningEndFields})
		cp.reasoningStarted = false
		// Advance so any text/reasoning block emitted after the tool returns
		// uses a fresh id (avoid id collision with the just-closed block).
		cp.advanceBlockID()
	}

	out = append(out, Chunk{Type: ChunkToolInputStart, Fields: map[string]any{
		"toolCallId": tcID,
		"toolName":   ev.ToolCallName,
	}})
	if ev.ToolCallArgsDelta != "" {
		out = append(out, Chunk{Type: ChunkToolInputDelta, Fields: map[string]any{
			"toolCallId":     tcID,
			"inputTextDelta": ev.ToolCallArgsDelta,
		}})
	}
	return out
}

func (cp *ChunkProducer) chunksToolCallDelta(ev aitypes.StepEvent) []Chunk {
	tcID := ev.ToolCallID
	if !cp.toolInputStarted[tcID] || ev.ToolCallArgsDelta == "" {
		return nil
	}
	existing := cp.toolArgsAccum[tcID]
	if isValidJSON(existing) {
		return nil
	}
	cp.toolArgsAccum[tcID] += ev.ToolCallArgsDelta
	return []Chunk{{Type: ChunkToolInputDelta, Fields: map[string]any{
		"toolCallId":     tcID,
		"inputTextDelta": ev.ToolCallArgsDelta,
	}}}
}

func (cp *ChunkProducer) chunksToolCallReady(ev aitypes.StepEvent) []Chunk {
	tcID := ev.ToolCallID
	if tcID == "" || cp.toolInputReady[tcID] {
		return nil
	}
	args := ev.ToolCallArgsDelta
	if args == "" {
		args = cp.toolArgsAccum[tcID]
	}
	cp.toolInputReady[tcID] = true
	return []Chunk{{Type: ChunkToolInputAvailable, Fields: withProviderMetadata(map[string]any{
		"toolCallId": tcID,
		"toolName":   ev.ToolCallName,
		"input":      parseToolArgs(args),
	}, ev.ProviderMetadata)}}
}

func (cp *ChunkProducer) chunksToolResult(ev aitypes.StepEvent) []Chunk {
	if ev.ToolResult == nil {
		return nil
	}
	tr := ev.ToolResult

	// Mirror ai-sdk-node's contract (packages/ai/src/generate-text/stream-text.ts
	// — `output: part.output` with schema `output: z.unknown()`): emit the
	// tool's output as-is. Tools in Go return `(string, error)`, so the raw
	// `tr.Output` is what flows through. Consumers re-parse with their own
	// typed shape; the stream layer no longer second-guesses.
	//
	// Why this shape matters: the previous behavior parsed `tr.Output` and
	// fell back to `{"result": tr.Output}` on parse failure, producing three
	// possible chunk shapes (parsed object / wrap object / re-stringified
	// string after persistence). Downstream code that expected a string
	// (e.g. SSE bridges that call `event["output"].(string)`) silently lost
	// the payload. Tools that return large structured outputs (image
	// generators, large search results) would also hit storage truncation
	// boundaries that mangled the JSON before parsing — same wrap fallback
	// fired, hiding the real shape from callers.
	//
	// `input` continues to parse `tr.Args` because tool args are reliably
	// JSON-emitted by the model and downstream UIs render structured fields.
	outputFields := withProviderMetadata(map[string]any{
		"toolCallId": tr.ID,
		"output":     tr.Output,
	}, ev.ProviderMetadata)

	chunks := make([]Chunk, 0, 2)
	if tr.ID != "" && !cp.toolInputReady[tr.ID] {
		cp.toolInputReady[tr.ID] = true
		chunks = append(chunks, Chunk{Type: ChunkToolInputAvailable, Fields: withProviderMetadata(map[string]any{
			"toolCallId": tr.ID,
			"toolName":   tr.Name,
			"input":      parseToolArgs(tr.Args),
		}, ev.ProviderMetadata)})
	}
	chunks = append(chunks, Chunk{Type: ChunkToolOutputAvailable, Fields: outputFields})
	return chunks
}

func parseToolArgs(args string) any {
	var parsed any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return map[string]string{"raw": args}
	}
	return parsed
}

func (cp *ChunkProducer) chunksToolCallInvalid(ev aitypes.StepEvent) []Chunk {
	return []Chunk{{Type: ChunkToolInputError, Fields: map[string]any{
		"toolCallId": ev.ToolCallID,
		"toolName":   ev.ToolCallName,
		"errorText":  fmt.Sprintf("invalid JSON arguments for tool %q", ev.ToolCallName),
	}}}
}

func (cp *ChunkProducer) chunksSource(ev aitypes.StepEvent) []Chunk {
	if ev.Source == nil || ev.Source.URL == "" {
		return nil
	}
	fields := map[string]any{
		"sourceId": ev.Source.ID,
		"url":      ev.Source.URL,
		"title":    ev.Source.Title,
	}
	return []Chunk{{Type: ChunkSourceURL, Fields: withProviderMetadata(fields, ev.Source.ProviderMetadata)}}
}

func (cp *ChunkProducer) chunksBlockEnd() []Chunk {
	var out []Chunk
	if cp.textStarted {
		out = append(out, Chunk{Type: ChunkTextEnd, Fields: map[string]any{"id": cp.textBlockID}})
	}
	if cp.reasoningStarted {
		reasoningEndFields := map[string]any{"id": cp.textBlockID}
		if cp.lastThoughtSignature != "" {
			reasoningEndFields["signature"] = cp.lastThoughtSignature
		}
		out = append(out, Chunk{Type: ChunkReasoningEnd, Fields: reasoningEndFields})
	}
	return out
}

func (cp *ChunkProducer) chunksStepEnd() []Chunk {
	out := cp.chunksBlockEnd()
	out = append(out, Chunk{Type: ChunkFinishStep, Fields: nil})
	return out
}

func (cp *ChunkProducer) chunksError(err error) []Chunk {
	out := cp.chunksBlockEnd()
	cp.textStarted = false
	cp.reasoningStarted = false

	// Redact unconditionally: raw provider error text can carry org/request
	// IDs and attacker-echoed content into the browser.
	out = append(out,
		Chunk{Type: ChunkError, Fields: map[string]any{"errorText": redactStreamError(err)}},
		Chunk{Type: ChunkFinish, Fields: map[string]any{"finishReason": "error"}},
	)
	cp.lastFinishReason = "error"
	return out
}

// blockID returns a text block identifier for step n.
func blockID(n int) string {
	return "text_" + itoa(n)
}

// itoa is a minimal int-to-string to avoid importing strconv for this one use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}

func isValidJSON(s string) bool {
	return json.Valid([]byte(s))
}

// withProviderMetadata attaches providerMetadata to a Fields map when pm is non-nil.
// Returns the (possibly newly allocated) map.
func withProviderMetadata(fields, pm map[string]any) map[string]any {
	if pm == nil {
		return fields
	}
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["providerMetadata"] = pm
	return fields
}

// MergeChunks merges multiple chunk channels into one output channel.
// Each source is drained concurrently; order within a single source is preserved
// but interleaving between sources is non-deterministic.
func MergeChunks(sources ...<-chan Chunk) <-chan Chunk {
	out := make(chan Chunk, 64)
	if len(sources) == 0 {
		close(out)
		return out
	}

	remaining := make(chan struct{}, len(sources))
	for _, src := range sources {
		go func() {
			// Signal completion even on panic so the closer below never
			// deadlocks waiting on a source that died mid-relay.
			defer func() { remaining <- struct{}{} }()
			defer safego.Recover(nil, recoverToChunk(out))
			for c := range src {
				out <- c
			}
		}()
	}

	go func() {
		// The only panic source here is a double close(out), which is
		// unreachable — so a bare boundary is correct: emitting onto out would
		// be self-defeating (out is the very channel being closed).
		defer safego.Recover(nil, nil)
		for i := 0; i < len(sources); i++ {
			<-remaining
		}
		close(out)
	}()

	return out
}
