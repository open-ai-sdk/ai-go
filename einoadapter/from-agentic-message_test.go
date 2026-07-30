package einoadapter

import (
	"errors"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/eino/schema/claude"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

// --- helpers ---

// chunk builds a streamed block at an index, mirroring the emission shape
// agenticclaude produces (event_convertor.go:254-326).
func chunk[T interface {
	schema.AssistantGenText | schema.Reasoning | schema.FunctionToolCall |
		schema.FunctionToolResult | schema.ServerToolCall
}](content *T, index int) *schema.ContentBlock {
	return schema.NewContentBlockChunk(content, &schema.StreamingMeta{Index: index})
}

// streamEvent wraps blocks as a streamed agent event, one block per frame — which is how
// a provider actually delivers them.
func streamEvent(blocks ...*schema.ContentBlock) *adk.TypedAgentEvent[*schema.AgenticMessage] {
	sr, sw := schema.Pipe[*schema.AgenticMessage](len(blocks) + 1)
	go func() {
		defer sw.Close()
		for _, b := range blocks {
			sw.Send(&schema.AgenticMessage{
				Role:          schema.AgenticRoleTypeAssistant,
				ContentBlocks: []*schema.ContentBlock{b},
			}, nil)
		}
	}()
	return &adk.TypedAgentEvent[*schema.AgenticMessage]{
		Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
				IsStreaming: true, MessageStream: sr,
				AgenticRole: schema.AgenticRoleTypeAssistant,
			},
		},
	}
}

func wholeEvent(blocks ...*schema.ContentBlock) *adk.TypedAgentEvent[*schema.AgenticMessage] {
	return &adk.TypedAgentEvent[*schema.AgenticMessage]{
		Output: &adk.TypedAgentOutput[*schema.AgenticMessage]{
			MessageOutput: &adk.TypedMessageVariant[*schema.AgenticMessage]{
				Message: &schema.AgenticMessage{
					Role: schema.AgenticRoleTypeAssistant, ContentBlocks: blocks,
				},
				AgenticRole: schema.AgenticRoleTypeAssistant,
			},
		},
	}
}

// collect drains the conversion.
func collect(t *testing.T, ev *adk.TypedAgentEvent[*schema.AgenticMessage], st *ConvState) []aisdk.StepEvent {
	t.Helper()
	var out []aisdk.StepEvent
	for e, err := range StepEventsFrom(ev, st) {
		if err != nil {
			t.Fatalf("conversion error: %v", err)
		}
		out = append(out, e)
	}
	return out
}

func types(evs []aisdk.StepEvent) []aisdk.StepEventType {
	out := make([]aisdk.StepEventType, len(evs))
	for i, e := range evs {
		out[i] = e.Type
	}
	return out
}

func assertTypes(t *testing.T, got []aisdk.StepEvent, want ...aisdk.StepEventType) {
	t.Helper()
	g := types(got)
	if len(g) != len(want) {
		t.Fatalf("event count: got %d %v, want %d %v", len(g), g, len(want), want)
	}
	for i := range want {
		if g[i] != want[i] {
			t.Fatalf("event %d: got %v, want %v\nfull: %v", i, g[i], want[i], g)
		}
	}
}

// --- the core property: streaming and non-streaming agree ---

// The v7 client requires text-start before text-delta whether or not the server streamed,
// so both paths must produce the same sequence for the same content. If they diverge, a
// non-streaming provider renders differently from a streaming one for no reason the user
// can see.
func TestStepEventsFrom_StreamingAndWholeAgree(t *testing.T) {
	cases := []struct {
		name   string
		blocks []*schema.ContentBlock
	}{
		{"text only", []*schema.ContentBlock{
			chunk(&schema.AssistantGenText{Text: "Hello world"}, 0),
		}},
		{"reasoning then text", []*schema.ContentBlock{
			chunk(&schema.Reasoning{Text: "thinking"}, 0),
			chunk(&schema.AssistantGenText{Text: "answer"}, 1),
		}},
		{"tool call", []*schema.ContentBlock{
			chunk(&schema.FunctionToolCall{CallID: "c1", Name: "echo", Arguments: `{"v":"x"}`}, 0),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			streamed := collect(t, streamEvent(tc.blocks...), NewConvState())
			whole := collect(t, wholeEvent(tc.blocks...), NewConvState())

			if len(streamed) != len(whole) {
				t.Fatalf("sequence length differs\n stream: %v\n whole:  %v",
					types(streamed), types(whole))
			}
			for i := range streamed {
				if streamed[i].Type != whole[i].Type {
					t.Errorf("event %d type: stream=%v whole=%v", i, streamed[i].Type, whole[i].Type)
				}
				if streamed[i].BlockID != whole[i].BlockID {
					t.Errorf("event %d blockID: stream=%q whole=%q",
						i, streamed[i].BlockID, whole[i].BlockID)
				}
				if streamed[i].TextDelta != whole[i].TextDelta {
					t.Errorf("event %d text: stream=%q whole=%q",
						i, streamed[i].TextDelta, whole[i].TextDelta)
				}
			}
		})
	}
}

func TestStepEventsFrom_TextLifecycle(t *testing.T) {
	got := collect(t, streamEvent(
		chunk(&schema.AssistantGenText{Text: "Hel"}, 0),
		chunk(&schema.AssistantGenText{Text: "lo"}, 0),
	), NewConvState())

	assertTypes(t, got,
		aisdk.StepEventTextStart, aisdk.StepEventTextDelta,
		aisdk.StepEventTextDelta, aisdk.StepEventTextEnd)

	for i, e := range got {
		if e.BlockID != "1-0" {
			t.Errorf("event %d blockID = %q, want 1-0", i, e.BlockID)
		}
	}
}

// A signature delta arrives with empty Text. Under a "same kind means delta" rule it
// becomes reasoning-delta{delta:""} and the signature — which some models require echoed
// back — is lost.
func TestStepEventsFrom_SignatureReachesReasoningEnd(t *testing.T) {
	got := collect(t, streamEvent(
		chunk(&schema.Reasoning{Text: "thinking..."}, 0),
		chunk(&schema.Reasoning{Signature: "sig-abc123"}, 0),
	), NewConvState())

	assertTypes(t, got,
		aisdk.StepEventReasoningStart, aisdk.StepEventReasoningDelta,
		aisdk.StepEventReasoningEnd)

	for _, e := range got {
		if e.Type == aisdk.StepEventReasoningDelta && e.ReasoningDelta == "" {
			t.Error("an empty reasoning-delta was emitted; the signature was flattened into it")
		}
	}
	end := got[len(got)-1]
	if end.ThoughtSignature != "sig-abc123" {
		t.Errorf("signature on reasoning-end = %q, want sig-abc123", end.ThoughtSignature)
	}
	if end.ProviderMetadata["signature"] != "sig-abc123" {
		t.Errorf("signature missing from reasoning-end provider metadata: %v", end.ProviderMetadata)
	}
}

// The spike measured that eino transports a signature with no preceding thinking delta,
// so the block must open lazily rather than assume one is already open.
func TestStepEventsFrom_SignatureWithoutPrecedingThinking(t *testing.T) {
	got := collect(t, streamEvent(
		chunk(&schema.Reasoning{Signature: "sig-orphan"}, 0),
	), NewConvState())

	assertTypes(t, got, aisdk.StepEventReasoningStart, aisdk.StepEventReasoningEnd)
	if got[1].ThoughtSignature != "sig-orphan" {
		t.Errorf("orphan signature lost: %+v", got[1])
	}
}

// A citations delta also has empty Text. It must not become text-delta{delta:""}.
func TestStepEventsFrom_CitationsDoNotBecomeEmptyTextDelta(t *testing.T) {
	citation := &claude.TextCitation{
		Type: claude.TextCitationTypeWebSearchResultLocation,
		WebSearchResultLocation: &claude.CitationWebSearchResultLocation{
			URL: "https://example.com/a", Title: "A",
		},
	}
	got := collect(t, streamEvent(
		chunk(&schema.AssistantGenText{Text: "grounded"}, 0),
		chunk(&schema.AssistantGenText{ClaudeExtension: &claude.AssistantGenTextExtension{
			Citations: []*claude.TextCitation{citation},
		}}, 0),
	), NewConvState())

	for _, e := range got {
		if e.Type == aisdk.StepEventTextDelta && e.TextDelta == "" {
			t.Error("an empty text-delta was emitted; the citation was flattened into it")
		}
	}
	var sawSource bool
	for _, e := range got {
		if e.Type == aisdk.StepEventSource {
			sawSource = true
			if e.Source == nil || e.Source.URL != "https://example.com/a" {
				t.Errorf("citation source = %+v", e.Source)
			}
		}
	}
	if !sawSource {
		t.Errorf("citation produced no source event: %v", types(got))
	}
}

// Delta frames carry no CallID, so identity has to come from the index. An empty
// toolCallId on a tool-input-delta makes the v7 client throw.
func TestStepEventsFrom_ToolCallDeltasNeverLoseTheCallID(t *testing.T) {
	got := collect(t, streamEvent(
		chunk(&schema.FunctionToolCall{CallID: "call_1", Name: "getWeather"}, 0),
		chunk(&schema.FunctionToolCall{Arguments: `{"city":`}, 0),
		chunk(&schema.FunctionToolCall{Arguments: `"Hanoi"}`}, 0),
	), NewConvState())

	assertTypes(t, got,
		aisdk.StepEventToolCallStart, aisdk.StepEventToolCallDelta,
		aisdk.StepEventToolCallDelta, aisdk.StepEventToolCallReady)

	for i, e := range got {
		if e.ToolCallID != "call_1" {
			t.Errorf("event %d (%v) toolCallID = %q, want call_1", i, e.Type, e.ToolCallID)
		}
	}
	// The fragments must reassemble.
	if input, _ := got[3].ToolInput.(string); input != `{"city":"Hanoi"}` {
		t.Errorf("assembled input = %q, want %q", input, `{"city":"Hanoi"}`)
	}
	if got[3].ToolCallName != "getWeather" {
		t.Errorf("tool name lost on ready: %q", got[3].ToolCallName)
	}
}

// ServerToolCall.CallID is documented as possibly empty, and two calls sharing "" would
// collapse into one rendered part without the client complaining.
func TestStepEventsFrom_ServerToolCallsGetDistinctSynthesizedIDs(t *testing.T) {
	got := collect(t, streamEvent(
		chunk(&schema.ServerToolCall{Name: "web_search", Arguments: map[string]any{"q": "a"}}, 0),
		chunk(&schema.ServerToolCall{Name: "web_search", Arguments: map[string]any{"q": "b"}}, 1),
	), NewConvState())

	ids := map[string]int{}
	for _, e := range got {
		if e.ToolCallID == "" {
			t.Errorf("a server tool event has an empty toolCallID: %+v", e)
		}
		if e.Type == aisdk.StepEventToolCallStart {
			ids[e.ToolCallID]++
		}
		if e.ProviderExecuted == nil || !*e.ProviderExecuted {
			t.Errorf("server tool event not marked providerExecuted: %+v", e)
		}
	}
	if len(ids) != 2 {
		t.Errorf("two server calls produced %d distinct ids: %v", len(ids), ids)
	}
	// No delta path: Arguments is `any`, not a partial-JSON string.
	for _, e := range got {
		if e.Type == aisdk.StepEventToolCallDelta {
			t.Error("a server tool call emitted an args delta; its arguments do not stream")
		}
	}
}

// Block ids must be unique across turns, because Eino restarts the index at 0 each turn
// and the v7 wire needs message-wide uniqueness.
func TestStepEventsFrom_BlockIDsUniqueAcrossThreeTurns(t *testing.T) {
	st := NewConvState()
	seen := map[string]bool{}

	for turn := 1; turn <= 3; turn++ {
		got := collect(t, streamEvent(
			chunk(&schema.AssistantGenText{Text: "turn text"}, 0),
			chunk(&schema.Reasoning{Text: "turn reasoning"}, 1),
		), st)

		for _, e := range got {
			if e.BlockID == "" {
				continue
			}
			if seen[e.BlockID] && e.Type != aisdk.StepEventTextEnd &&
				e.Type != aisdk.StepEventReasoningEnd &&
				e.Type != aisdk.StepEventTextDelta &&
				e.Type != aisdk.StepEventReasoningDelta {
				t.Errorf("turn %d reused block id %q for a start event", turn, e.BlockID)
			}
			seen[e.BlockID] = true
		}
	}
	// 3 turns x 2 blocks.
	if len(seen) != 6 {
		t.Errorf("got %d distinct block ids across 3 turns, want 6: %v", len(seen), keysOf(seen))
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Interleaved reasoning and text keep distinct indices and must not merge.
func TestStepEventsFrom_InterleavedReasoningAndText(t *testing.T) {
	got := collect(t, streamEvent(
		chunk(&schema.Reasoning{Text: "Let me "}, 0),
		chunk(&schema.Reasoning{Text: "think."}, 0),
		chunk(&schema.AssistantGenText{Text: "Answer"}, 1),
		chunk(&schema.Reasoning{Text: "More."}, 2),
		chunk(&schema.AssistantGenText{Text: " done"}, 3),
	), NewConvState())

	assertTypes(t, got,
		aisdk.StepEventReasoningStart, aisdk.StepEventReasoningDelta, aisdk.StepEventReasoningDelta,
		aisdk.StepEventTextStart, aisdk.StepEventTextDelta,
		aisdk.StepEventReasoningStart, aisdk.StepEventReasoningDelta,
		aisdk.StepEventTextStart, aisdk.StepEventTextDelta,
		// All four blocks flushed at end of stream, in index order.
		aisdk.StepEventReasoningEnd, aisdk.StepEventTextEnd,
		aisdk.StepEventReasoningEnd, aisdk.StepEventTextEnd)
}

// Every still-open block must close when the stream ends, or the client's part stays
// state:"streaming" forever — finish-step resets its active-part maps.
func TestStepEventsFrom_ClosesOpenBlocksAtEndOfStream(t *testing.T) {
	got := collect(t, streamEvent(
		chunk(&schema.AssistantGenText{Text: "unterminated"}, 0),
		chunk(&schema.Reasoning{Text: "also unterminated"}, 1),
	), NewConvState())

	var textEnds, reasoningEnds int
	for _, e := range got {
		switch e.Type {
		case aisdk.StepEventTextEnd:
			textEnds++
		case aisdk.StepEventReasoningEnd:
			reasoningEnds++
		}
	}
	if textEnds != 1 || reasoningEnds != 1 {
		t.Errorf("open blocks not closed: %d text-end, %d reasoning-end in %v",
			textEnds, reasoningEnds, types(got))
	}
}

// And on the error path too.
func TestStepEventsFrom_ClosesOpenBlocksOnError(t *testing.T) {
	st := NewConvState()
	// Open a block, then hand the converter an event carrying an error.
	_ = collect(t, streamEvent(chunk(&schema.AssistantGenText{Text: "partial"}, 0)), st)

	// Re-open without flushing, to simulate a mid-block failure.
	st.BeginTurn()
	st.Open(0, blockText)

	var got []aisdk.StepEvent
	for e, err := range StepEventsFrom(&adk.TypedAgentEvent[*schema.AgenticMessage]{
		Err: errors.New("provider exploded"),
	}, st) {
		if err != nil {
			t.Fatalf("unexpected iteration error: %v", err)
		}
		got = append(got, e)
	}

	assertTypes(t, got, aisdk.StepEventTextEnd, aisdk.StepEventError)
	if got[1].Error == nil {
		t.Error("error event carries no error")
	}
}

func TestStepEventsFrom_InterruptClosesBlocksAndSurfaces(t *testing.T) {
	st := NewConvState()
	st.BeginTurn()
	st.Open(0, blockText)

	info := &adk.InterruptInfo{}
	var got []aisdk.StepEvent
	for e := range StepEventsFrom(&adk.TypedAgentEvent[*schema.AgenticMessage]{
		Action: &adk.AgentAction{Interrupted: info},
	}, st) {
		got = append(got, e)
	}

	assertTypes(t, got, aisdk.StepEventTextEnd, aisdk.StepEventInterrupted)
	if got[1].Interrupt == nil {
		t.Error("interrupt not surfaced for the approval gate")
	}
}

func TestStepEventsFrom_EmptyAndNilInputs(t *testing.T) {
	if got := collect(t, nil, NewConvState()); len(got) != 0 {
		t.Errorf("nil event produced %v", types(got))
	}
	if got := collect(t, &adk.TypedAgentEvent[*schema.AgenticMessage]{}, NewConvState()); len(got) != 0 {
		t.Errorf("empty event produced %v", types(got))
	}
	if got := collect(t, streamEvent(), NewConvState()); len(got) != 0 {
		t.Errorf("empty stream produced %v", types(got))
	}
}

func TestStepEventsFrom_ToolResultStructuralConversion(t *testing.T) {
	got := collect(t, wholeEvent(schema.NewContentBlock(&schema.FunctionToolResult{
		CallID: "c1", Name: "echo",
		Content: []*schema.FunctionToolResultContentBlock{
			{Type: schema.FunctionToolResultContentBlockTypeText,
				Text: &schema.UserInputText{Text: "hello "}},
			{Type: schema.FunctionToolResultContentBlockTypeText,
				Text: &schema.UserInputText{Text: "world"}},
			{Type: schema.FunctionToolResultContentBlockTypeImage,
				Image: &schema.UserInputImage{MIMEType: "image/png"}},
		},
	})), NewConvState())

	assertTypes(t, got, aisdk.StepEventToolResult)
	res := got[0].ToolResult
	if res == nil {
		t.Fatal("no tool result")
	}
	// Content is a list of typed blocks, not a scalar — text parts join, media is kept
	// separately rather than stringified into the output.
	if res.Output != "hello world" {
		t.Errorf("output = %q, want %q", res.Output, "hello world")
	}
	if len(res.Content) != 3 {
		t.Errorf("content parts = %d, want 3: %+v", len(res.Content), res.Content)
	}
}

// mcp_* and assistant_gen_image are inbound-rejected by the provider, so emitting them
// would put content on the wire that cannot survive the next turn's history round-trip.
func TestStepEventsFrom_DropsInboundRejectedBlockKinds(t *testing.T) {
	for _, b := range []*schema.ContentBlock{
		schema.NewContentBlock(&schema.MCPToolCall{Name: "x", CallID: "m1"}),
		schema.NewContentBlock(&schema.MCPToolResult{CallID: "m1"}),
		schema.NewContentBlock(&schema.AssistantGenImage{MIMEType: "image/png", Base64Data: "aa"}),
		schema.NewContentBlock(&schema.AssistantGenAudio{MIMEType: "audio/wav"}),
	} {
		got := collect(t, wholeEvent(b), NewConvState())
		if len(got) != 0 {
			t.Errorf("%s produced %v, want nothing", b.Type, types(got))
		}
	}
}

func TestStepEventsFrom_UsageFromResponseMeta(t *testing.T) {
	ev := wholeEvent(chunk(&schema.AssistantGenText{Text: "hi"}, 0))
	ev.Output.MessageOutput.Message.ResponseMeta = &schema.AgenticResponseMeta{
		TokenUsage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}

	got := collect(t, ev, NewConvState())
	last := got[len(got)-1]
	if last.Type != aisdk.StepEventUsage {
		t.Fatalf("last event = %v, want usage: %v", last.Type, types(got))
	}
	if last.Usage == nil || last.Usage.TotalTokens != 15 {
		t.Errorf("usage = %+v", last.Usage)
	}
}

// A consumer that stops early must not wedge the converter.
func TestStepEventsFrom_EarlyBreakIsSafe(t *testing.T) {
	st := NewConvState()
	var n int
	for range StepEventsFrom(streamEvent(
		chunk(&schema.AssistantGenText{Text: "a"}, 0),
		chunk(&schema.AssistantGenText{Text: "b"}, 0),
	), st) {
		n++
		break
	}
	if n != 1 {
		t.Errorf("consumed %d events after break, want 1", n)
	}
}
