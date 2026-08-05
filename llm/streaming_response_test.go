package llm_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

// countingStreamModel records how many provider calls were made so lazy start
// is observable.
type countingStreamModel struct {
	streamCalls int
	events      []aikit.StreamEvent
	err         error
	nilStream   bool
}

func (*countingStreamModel) ModelID() string { return "counting-stream-test" }

func (m *countingStreamModel) Stream(context.Context, llm.Request) (<-chan aikit.StreamEvent, error) {
	m.streamCalls++
	if m.err != nil {
		return nil, m.err
	}
	if m.nilStream {
		return nil, nil
	}
	ch := make(chan aikit.StreamEvent, len(m.events))
	for _, event := range m.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

// richEvents exercises every CompletionResponse field a stream can populate.
func richEvents() []aikit.StreamEvent {
	return []aikit.StreamEvent{
		{Type: aikit.StreamEventReasoningDelta, TextDelta: "think", ThoughtSignature: "sig"},
		{Type: aikit.StreamEventTextDelta, TextDelta: "hel"},
		{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallID: "tc1", ToolCallName: "search"},
		{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallArgsDelta: `{"q":`},
		{Type: aikit.StreamEventTextDelta, TextDelta: "lo"},
		{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallArgsDelta: `"go"}`},
		{Type: aikit.StreamEventSource, Source: &aikit.Source{
			SourceType: "url", ID: "s1", URL: "https://example.test",
		}},
		{Type: aikit.StreamEventFileDelta, FileData: []byte("png-bytes"), FileMediaType: "image/png"},
		{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{InputTokens: 11, TotalTokens: 11}},
		{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{OutputTokens: 4}},
		{
			Type:             aikit.StreamEventFinish,
			MessageID:        "msg-1",
			FinishReason:     aikit.FinishReasonToolCalls,
			RawFinishReason:  "tool_calls",
			ProviderMetadata: map[string]any{"openai": "meta"},
			Warnings:         []aikit.Warning{{Type: "other", Message: "careful"}},
		},
	}
}

func drain(t *testing.T, stream *llm.StreamingResponse) []aikit.StreamEvent {
	t.Helper()
	var seen []aikit.StreamEvent
	for event, err := range stream.Events() {
		if err != nil {
			t.Fatalf("Events() error = %v", err)
		}
		seen = append(seen, event)
	}
	return seen
}

// wantRichResponse is what richEvents must aggregate to, written out rather
// than derived. Comparing the aggregate to Send would prove nothing now that
// Send is StreamSend plus Response.
func wantRichResponse() *llm.CompletionResponse {
	return &llm.CompletionResponse{
		Message: aikit.Message{
			ID:   "msg-1",
			Role: aikit.RoleAssistant,
			Content: []aikit.ContentPart{
				{Type: aikit.ContentPartTypeReasoning, ReasoningText: "think", ThoughtSignature: "sig"},
				{Type: aikit.ContentPartTypeText, Text: "hel"},
				{
					Type: aikit.ContentPartTypeToolCall, ToolCallID: "tc1",
					ToolCallName: "search", ToolCallArgs: []byte(`{"q":"go"}`),
				},
				{Type: aikit.ContentPartTypeText, Text: "lo"},
				{Type: aikit.ContentPartTypeFile, Data: []byte("png-bytes"), MediaType: "image/png"},
			},
		},
		MessageID: "msg-1",
		Text:      "hello",
		Reasoning: "think",
		// The two usage events merge rather than sum: the second report carries
		// only OutputTokens, and its zero input count must not clobber the 11
		// from the first.
		Usage:            aikit.Usage{InputTokens: 11, OutputTokens: 4, TotalTokens: 11},
		FinishReason:     aikit.FinishReasonToolCalls,
		RawFinishReason:  "tool_calls",
		ProviderMetadata: map[string]any{"openai": "meta"},
		Warnings:         []aikit.Warning{{Type: "other", Message: "careful"}},
		Sources: []aikit.Source{
			{SourceType: "url", ID: "s1", URL: "https://example.test"},
		},
		Files: []llm.GeneratedFile{{Data: []byte("png-bytes"), MediaType: "image/png"}},
	}
}

// The aggregate a drained stream exposes must be every field Send used to
// build, asserted against a written-out expectation.
func TestStreamingResponseAggregatesEveryField(t *testing.T) {
	stream, err := llm.NewCompletion(&countingStreamModel{events: richEvents()}, "prompt").
		StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	drain(t, stream)
	streamed, err := stream.Response()
	if err != nil {
		t.Fatalf("Response() error = %v", err)
	}

	if !reflect.DeepEqual(streamed, wantRichResponse()) {
		t.Fatalf("Response() = %#v,\nwant %#v", streamed, wantRichResponse())
	}
}

// Send is now StreamSend plus Response, so this is a consistency check on the
// rewire rather than independent proof of the aggregate.
func TestSendAgreesWithTheDrainedStream(t *testing.T) {
	sent, err := llm.NewCompletion(&countingStreamModel{events: richEvents()}, "prompt").Send(context.Background())
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !reflect.DeepEqual(sent, wantRichResponse()) {
		t.Fatalf("Send() = %#v,\nwant %#v", sent, wantRichResponse())
	}
}

// Tool calls must keep the position where their first delta arrived, so text
// emitted around them stays in order.
func TestStreamingResponseKeepsToolCallPositionAmongText(t *testing.T) {
	stream, err := llm.NewCompletion(&countingStreamModel{events: richEvents()}, "prompt").
		StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	drain(t, stream)
	response, err := stream.Response()
	if err != nil {
		t.Fatalf("Response() error = %v", err)
	}

	types := make([]aikit.ContentPartType, 0, len(response.Message.Content))
	for _, part := range response.Message.Content {
		types = append(types, part.Type)
	}
	// Text emitted after the tool call opens a second text part rather than
	// extending the one before it, which is what pins the call's position.
	want := []aikit.ContentPartType{
		aikit.ContentPartTypeReasoning,
		aikit.ContentPartTypeText,
		aikit.ContentPartTypeToolCall,
		aikit.ContentPartTypeText,
		aikit.ContentPartTypeFile,
	}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("content part types = %v, want %v", types, want)
	}
	call := response.Message.Content[2]
	if call.ToolCallID != "tc1" || call.ToolCallName != "search" {
		t.Errorf("tool call identity = (%q, %q), want (tc1, search)", call.ToolCallID, call.ToolCallName)
	}
	if string(call.ToolCallArgs) != `{"q":"go"}` {
		t.Errorf("ToolCallArgs = %q, want %q", call.ToolCallArgs, `{"q":"go"}`)
	}
}

func TestStreamingResponseIsSingleUse(t *testing.T) {
	stream, err := llm.NewCompletion(&countingStreamModel{events: richEvents()}, "prompt").
		StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	drain(t, stream)

	var secondErr error
	var count int
	for _, err := range stream.Events() {
		count++
		secondErr = err
	}
	if count != 1 || !errors.Is(secondErr, llm.ErrStreamUsed) {
		t.Fatalf("second range yielded %d values, last error %v, want 1 and ErrStreamUsed", count, secondErr)
	}
}

func TestStreamingResponseRejectsReadBeforeDrain(t *testing.T) {
	stream, err := llm.NewCompletion(&countingStreamModel{events: richEvents()}, "prompt").
		StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	if stream.State() != llm.StreamNotDrained {
		t.Errorf("State() = %v, want StreamNotDrained", stream.State())
	}
	response, err := stream.Response()
	if !errors.Is(err, llm.ErrStreamNotDrained) {
		t.Fatalf("Response() error = %v, want ErrStreamNotDrained", err)
	}
	if response != nil {
		t.Errorf("Response() = %#v, want nil before drain", response)
	}
}

// A consumer that stops on the provider's terminal event has the whole
// aggregate; treating that as a cancellation would report a failure that did
// not happen.
func TestStreamingResponseBreakOnFinalEventCompletes(t *testing.T) {
	stream, err := llm.NewCompletion(&countingStreamModel{events: richEvents()}, "prompt").
		StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	for event, err := range stream.Events() {
		if err != nil {
			t.Fatalf("Events() error = %v", err)
		}
		if event.Type == aikit.StreamEventFinish {
			break
		}
	}

	if stream.State() != llm.StreamCompleted {
		t.Errorf("State() = %v, want StreamCompleted", stream.State())
	}
	response, err := stream.Response()
	if err != nil {
		t.Fatalf("Response() error = %v, want nil — no cancellation happened", err)
	}
	if response.Text != "hello" || response.FinishReason != aikit.FinishReasonToolCalls {
		t.Errorf("response = %#v, want the complete aggregate", response)
	}
}

// StreamEventFinish is not always the last event. OpenAI-compatible endpoints
// report usage on a trailing chunk with no choices, and the Gemini native
// decoder does the same, so breaking on finish silently drops those counts.
// StreamCompleted means nothing failed, not that everything was seen.
func TestStreamingResponseBreakOnFinishDropsTrailingUsage(t *testing.T) {
	trailingUsage := []aikit.StreamEvent{
		{Type: aikit.StreamEventTextDelta, TextDelta: "hello"},
		{Type: aikit.StreamEventFinish, MessageID: "m1", FinishReason: aikit.FinishReasonStop},
		{Type: aikit.StreamEventUsage, Usage: &aikit.Usage{InputTokens: 7, OutputTokens: 3, TotalTokens: 10}},
	}

	drained, err := llm.NewCompletion(&countingStreamModel{events: trailingUsage}, "prompt").
		Send(context.Background())
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if drained.Usage.TotalTokens != 10 {
		t.Fatalf("fully drained TotalTokens = %d, want 10", drained.Usage.TotalTokens)
	}

	stream, err := llm.NewCompletion(&countingStreamModel{events: trailingUsage}, "prompt").
		StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	for event, err := range stream.Events() {
		if err != nil {
			t.Fatalf("Events() error = %v", err)
		}
		if event.Type == aikit.StreamEventFinish {
			break
		}
	}

	if stream.State() != llm.StreamCompleted {
		t.Errorf("State() = %v, want StreamCompleted — nothing failed", stream.State())
	}
	partial, err := stream.Response()
	if err != nil {
		t.Fatalf("Response() error = %v, want nil", err)
	}
	if partial.Usage.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0 — the trailing usage event was never yielded",
			partial.Usage.TotalTokens)
	}
	if partial.Text != "hello" || partial.FinishReason != aikit.FinishReasonStop {
		t.Errorf("partial = %#v, want everything up to and including finish", partial)
	}
}

func TestStreamingResponseBreakBeforeFinalEventAborts(t *testing.T) {
	stream, err := llm.NewCompletion(&countingStreamModel{events: richEvents()}, "prompt").
		StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	for event := range stream.Events() {
		if event.Type == aikit.StreamEventTextDelta {
			break
		}
	}

	if stream.State() != llm.StreamAborted {
		t.Errorf("State() = %v, want StreamAborted", stream.State())
	}
	response, err := stream.Response()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Response() error = %v, want context.Canceled", err)
	}
	if response == nil {
		t.Fatal("Response() = nil with a non-nil error, want the partial aggregate")
	}
	if response.Reasoning != "think" || response.Text != "hel" {
		t.Errorf("partial aggregate = %#v, want everything folded before the break", response)
	}
}

// Lazy start is what keeps an unranged response from leaking a provider
// connection and its transport goroutine.
func TestStreamingResponseDoesNotCallProviderUntilRanged(t *testing.T) {
	model := &countingStreamModel{events: richEvents()}
	stream, err := llm.NewCompletion(model, "prompt").StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	if model.streamCalls != 0 {
		t.Fatalf("streamCalls = %d before ranging, want 0", model.streamCalls)
	}
	drain(t, stream)
	if model.streamCalls != 1 {
		t.Fatalf("streamCalls = %d after ranging, want 1", model.streamCalls)
	}
}

func TestStreamingResponseCloseReleasesUnrangedStream(t *testing.T) {
	model := &countingStreamModel{events: richEvents()}
	stream, err := llm.NewCompletion(model, "prompt").StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if model.streamCalls != 0 {
		t.Errorf("streamCalls = %d after Close on an unranged stream, want 0", model.streamCalls)
	}

	var closedErr error
	for _, err := range stream.Events() {
		closedErr = err
	}
	if !errors.Is(closedErr, llm.ErrStreamUsed) {
		t.Errorf("Events() after Close error = %v, want ErrStreamUsed", closedErr)
	}
}

func TestStreamingResponseCloseAfterDrainIsNoOp(t *testing.T) {
	stream, err := llm.NewCompletion(&countingStreamModel{events: richEvents()}, "prompt").
		StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	drain(t, stream)
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if stream.State() != llm.StreamCompleted {
		t.Errorf("State() = %v after Close on a drained stream, want StreamCompleted", stream.State())
	}
	if _, err := stream.Response(); err != nil {
		t.Errorf("Response() after Close error = %v, want nil", err)
	}
}

// Request validation stays synchronous; only the provider call is deferred.
func TestStreamSendReportsValidationErrorImmediately(t *testing.T) {
	stream, err := llm.NewCompletion(nil, "prompt").StreamSend(context.Background())
	if err == nil {
		t.Fatal("StreamSend() error = nil for a nil model, want a request error")
	}
	if stream != nil {
		t.Errorf("StreamSend() = %#v on the error path, want a nil pointer", stream)
	}
	var completionErr *llm.CompletionError
	if !errors.As(err, &completionErr) || completionErr.Kind != llm.CompletionErrorKindRequest {
		t.Fatalf("error = %v, want a CompletionError of kind invalid_request", err)
	}
}

func TestStreamingResponseSurfacesProviderStartFailureOnFirstPull(t *testing.T) {
	model := &countingStreamModel{err: errors.New("dial failed")}
	stream, err := llm.NewCompletion(model, "prompt").StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v, want the provider call deferred", err)
	}

	var pullErr error
	for _, err := range stream.Events() {
		pullErr = err
	}
	var completionErr *llm.CompletionError
	if !errors.As(pullErr, &completionErr) || completionErr.Operation != "stream" {
		t.Fatalf("first-pull error = %v, want a CompletionError with Operation stream", pullErr)
	}
	response, err := stream.Response()
	if response != nil {
		t.Errorf("Response() = %#v, want nil when the stream never opened", response)
	}
	if !errors.As(err, &completionErr) || completionErr.Operation != "stream" {
		t.Errorf("Response() error = %v, want the construction error", err)
	}
}

func TestStreamingResponseRejectsNilProviderStream(t *testing.T) {
	stream, err := llm.NewCompletion(&countingStreamModel{nilStream: true}, "prompt").
		StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}
	var pullErr error
	for _, err := range stream.Events() {
		pullErr = err
	}
	var completionErr *llm.CompletionError
	if !errors.As(pullErr, &completionErr) || completionErr.Kind != llm.CompletionErrorKindResponse {
		t.Fatalf("error = %v, want a CompletionError of kind invalid_response", pullErr)
	}
}

// A provider error event reaches the consumer through the error half of the
// sequence and leaves the partial aggregate readable.
func TestStreamingResponseDeliversProviderErrorEventThroughIterator(t *testing.T) {
	failure := errors.New("provider exploded")
	model := &countingStreamModel{events: []aikit.StreamEvent{
		{Type: aikit.StreamEventTextDelta, TextDelta: "partial"},
		{Type: aikit.StreamEventError, Error: failure},
	}}
	stream, err := llm.NewCompletion(model, "prompt").StreamSend(context.Background())
	if err != nil {
		t.Fatalf("StreamSend() error = %v", err)
	}

	var seenErr error
	for _, err := range stream.Events() {
		if err != nil {
			seenErr = err
		}
	}
	if !errors.Is(seenErr, failure) {
		t.Fatalf("Events() error = %v, want the provider error", seenErr)
	}
	if stream.State() != llm.StreamAborted {
		t.Errorf("State() = %v, want StreamAborted", stream.State())
	}
	response, err := stream.Response()
	if !errors.Is(err, failure) {
		t.Fatalf("Response() error = %v, want the provider error", err)
	}
	if response == nil || response.Text != "partial" {
		t.Errorf("Response() = %#v, want the partial aggregate", response)
	}
}

// Send's failure shape is a public contract: the same error kind, the same
// collect operation, and the partial response alongside it.
func TestSendKeepsCollectErrorShapeAndPartialResponse(t *testing.T) {
	failure := errors.New("provider exploded")
	model := &countingStreamModel{events: []aikit.StreamEvent{
		{Type: aikit.StreamEventTextDelta, TextDelta: "partial"},
		{Type: aikit.StreamEventError, Error: failure},
	}}
	response, err := llm.NewCompletion(model, "prompt").Send(context.Background())

	var completionErr *llm.CompletionError
	if !errors.As(err, &completionErr) {
		t.Fatalf("Send() error = %v, want a CompletionError", err)
	}
	if completionErr.Operation != "collect" {
		t.Errorf("Operation = %q, want collect", completionErr.Operation)
	}
	if completionErr.Kind != llm.CompletionErrorKindProvider {
		t.Errorf("Kind = %q, want provider", completionErr.Kind)
	}
	if !errors.Is(err, failure) {
		t.Errorf("error does not unwrap to the provider error: %v", err)
	}
	if response == nil || response.Text != "partial" {
		t.Fatalf("Send() response = %#v, want the partial aggregate", response)
	}
}

// A construction failure has no aggregate, so Send must still return a nil
// response and keep the stream operation on the error.
func TestSendReturnsNilResponseWhenProviderStreamNeverOpens(t *testing.T) {
	model := &countingStreamModel{err: errors.New("dial failed")}
	response, err := llm.NewCompletion(model, "prompt").Send(context.Background())

	if response != nil {
		t.Errorf("Send() response = %#v, want nil", response)
	}
	var completionErr *llm.CompletionError
	if !errors.As(err, &completionErr) || completionErr.Operation != "stream" {
		t.Fatalf("Send() error = %v, want a CompletionError with Operation stream", err)
	}
}

// The unified fold gave direct completions the stop-after-valid-JSON gate, so a
// provider that re-sends complete arguments no longer produces invalid JSON.
func TestStreamingResponseIgnoresToolArgumentsAfterValidJSON(t *testing.T) {
	model := &countingStreamModel{events: []aikit.StreamEvent{
		{
			Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0,
			ToolCallID: "tc1", ToolCallName: "echo", ToolCallArgsDelta: `{"a":1}`,
		},
		{Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0, ToolCallArgsDelta: `{"a":1}`},
		{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonToolCalls},
	}}
	response, err := llm.NewCompletion(model, "prompt").Send(context.Background())
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if got := string(response.Message.Content[0].ToolCallArgs); got != `{"a":1}` {
		t.Fatalf("ToolCallArgs = %q, want %q", got, `{"a":1}`)
	}
}
