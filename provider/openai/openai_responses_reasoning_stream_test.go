package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/transport"
)

func reasoningStreamEvents(t *testing.T, enabled bool, chunks ...string) []aikit.StreamEvent {
	t.Helper()
	var body strings.Builder
	for _, chunk := range chunks {
		body.WriteString("data: " + chunk + "\n\n")
	}
	ch := make(chan aikit.StreamEvent, 128)
	if err := decodeResponsesSSEStreamWithOptions(
		context.Background(),
		transport.NewSSEReader(strings.NewReader(body.String())),
		ch,
		"req_test",
		enabled,
		true, // These tests intentionally decode isolated item/part fragments.
	); err != nil {
		t.Fatal(err)
	}
	close(ch)
	var events []aikit.StreamEvent
	for event := range ch {
		events = append(events, event)
	}
	return events
}

func TestResponsesReasoningLifecycleOptIn(t *testing.T) {
	chunks := []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs.raw","summary":[]}}`,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"item_id":"rs.raw","summary_index":0,"delta":" live "}`,
		`{"type":"response.reasoning_summary_text.done","output_index":0,"item_id":"rs.raw","summary_index":0,"text":" live "}`,
		`{"type":"response.reasoning_summary_part.done","output_index":0,"item_id":"rs.raw","summary_index":0,"part":{"type":"summary_text","text":" live "}}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs.raw","encrypted_content":"ciphertext","content":[{"type":"text","text":"private"}],"summary":[{"type":"summary_text","text":"  First\nline  "},{"type":"encrypted","text":"secret"},{"type":"redacted","text":"redacted"},{"type":"text","text":"not a summary"},{"type":"summary_text","text":" \t\n"},{"type":"summary_text","text":"\u2003Second\u2003"}]}}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs.raw","summary":[{"type":"summary_text","text":"duplicate"}]}}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"reasoning","id":"rs.empty","summary":[],"encrypted_content":"hidden"}}`,
		`{"type":"response.output_text.delta","delta":"answer"}`,
		`{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[{"type":"reasoning","id":"rs.raw","summary":[{"type":"summary_text","text":"duplicate"}]},{"type":"reasoning","id":"rs.empty","summary":[]}]}}`,
	}
	baseline := reasoningStreamEvents(t, false, chunks...)
	if len(baseline) != 3 || baseline[0].Type != aikit.StreamEventReasoningDelta || baseline[0].TextDelta != " live " ||
		baseline[0].ReasoningID != nil ||
		baseline[1].Type != aikit.StreamEventTextDelta ||
		baseline[2].Type != aikit.StreamEventFinish {
		t.Fatalf("legacy events changed: %#v", baseline)
	}
	events := reasoningStreamEvents(t, true, chunks...)
	if len(events) != 5 || events[0].ReasoningID == nil || *events[0].ReasoningID != "rs.raw" {
		t.Fatalf("opt-in events: %#v", events)
	}
	for i, want := range []string{"First\nline\n\nSecond", ""} {
		event := events[i+1]
		if event.Type != aikit.StreamEventReasoningSummary || event.ReasoningSummary != want ||
			event.ReasoningID == nil {
			t.Fatalf("summary %d: %#v", i, event)
		}
	}
	if *events[1].ReasoningID != "rs.raw" || *events[2].ReasoningID != "rs.empty" {
		t.Fatalf("terminal source IDs: %#v", events)
	}
	// Removing only the opt-in additions recovers every original event exactly.
	var legacy []aikit.StreamEvent
	for _, event := range events {
		if event.Type == aikit.StreamEventReasoningSummary {
			continue
		}
		event.ReasoningID = nil
		legacy = append(legacy, event)
	}
	legacyJSON, legacyErr := json.Marshal(legacy)
	baselineJSON, baselineErr := json.Marshal(baseline)
	if legacyErr != nil || baselineErr != nil || string(legacyJSON) != string(baselineJSON) {
		t.Fatalf("legacy stream changed: %#v != %#v", legacy, baseline)
	}
}

func TestResponsesReasoningRawIDPresence(t *testing.T) {
	for _, tc := range []struct {
		name, deltaID, itemID string
		want                  *string
	}{
		{name: "missing"},
		{name: "null", deltaID: `,"item_id":null`, itemID: `,"id":null`},
		{name: "empty", deltaID: `,"item_id":""`, itemID: `,"id":""`, want: stringPointer("")},
		{name: "raw", deltaID: `,"item_id":" rs:0 /raw "`, itemID: `,"id":" rs:0 /raw "`, want: stringPointer(" rs:0 /raw ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := reasoningStreamEvents(
				t,
				true,
				`{"type":"response.output_item.added","output_index":7,"item":{"type":"reasoning"`+tc.itemID+`}}`,
				`{"type":"response.reasoning_summary_text.delta","output_index":7,"delta":"x"`+tc.deltaID+`}`,
				`{"type":"response.output_item.done","output_index":7,"item":{"type":"reasoning","summary":[]`+tc.itemID+`}}`,
			)
			if len(events) != 2 || events[0].Type != aikit.StreamEventReasoningDelta ||
				events[1].Type != aikit.StreamEventReasoningSummary {
				t.Fatalf("events: %#v", events)
			}
			for _, event := range events {
				if !reflect.DeepEqual(event.ReasoningID, tc.want) {
					t.Fatalf("raw ID = %#v, want %#v", event.ReasoningID, tc.want)
				}
			}
		})
	}
	// Correlation must not manufacture an item_id absent from a delta.
	events := reasoningStreamEvents(t, true,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"known"}}`,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"delta":"x"}`,
	)
	if len(events) != 1 || events[0].ReasoningID != nil {
		t.Fatalf("inferred a missing source ID: %#v", events)
	}
}

func stringPointer(value string) *string { return &value }

func TestResponsesReasoningPartCompletionIsNotItemCompletion(t *testing.T) {
	chunks := []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[]}}`,
		`{"type":"response.reasoning_summary_part.added","output_index":0,"item_id":"rs_1","summary_index":2,"part":{"type":"summary_text","text":""}}`,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"item_id":"rs_1","summary_index":2,"delta":"partial"}`,
		`{"type":"response.reasoning_summary_text.done","output_index":0,"item_id":"rs_1","summary_index":2,"text":" Second "}`,
		`{"type":"response.reasoning_summary_part.done","output_index":0,"item_id":"rs_1","summary_index":2,"part":{"type":"summary_text","text":" Second "}}`,
		`{"type":"response.reasoning_summary_part.done","output_index":0,"item_id":"rs_1","summary_index":0,"part":{"type":"summary_text","text":" First "}}`,
		`{"type":"response.reasoning_summary_part.done","output_index":0,"item_id":"rs_1","summary_index":1,"part":{"type":"redacted","text":"secret"}}`,
	}
	initialEvents := reasoningStreamEvents(t, true, chunks...)
	if len(initialEvents) != 1 || initialEvents[0].Type != aikit.StreamEventReasoningDelta {
		t.Fatalf("premature terminal event: %#v", initialEvents)
	}
	chunks = append(
		chunks,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1"}}`,
	)
	events := reasoningStreamEvents(t, true, chunks...)
	if len(events) != 2 || events[1].ReasoningSummary != "First\n\nSecond" {
		t.Fatalf("summary parts were duplicated or reordered: %#v", events)
	}
	// An explicitly empty final summary overrides any accumulated part text.
	chunks[len(chunks)-1] = `{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[]}}`
	events = reasoningStreamEvents(t, true, chunks...)
	if len(events) != 2 || events[1].ReasoningSummary != "" {
		t.Fatalf("final summary was not authoritative: %#v", events)
	}
}

func TestResponsesReasoningCompletedOutputAndNoSyntheticCompletion(t *testing.T) {
	events := reasoningStreamEvents(
		t,
		true,
		`{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","id":"msg_1"},{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":" final "}]},{"type":"reasoning","summary":[{"type":"encrypted","text":"secret"}]}]}}`,
	)
	if len(events) != 3 || events[0].ReasoningSummary != "final" ||
		events[1].Type != aikit.StreamEventReasoningSummary ||
		events[1].ReasoningSummary != "" ||
		events[1].ReasoningID != nil ||
		events[2].Type != aikit.StreamEventFinish {
		t.Fatalf("completed output: %#v", events)
	}
	for _, ending := range []string{
		`{"type":"error","message":"failed"}`,
		`{"type":"response.failed","response":{"status":"failed"}}`,
		`{"type":"response.incomplete","response":{"status":"incomplete"}}`,
		`[DONE]`,
	} {
		events := reasoningStreamEvents(
			t,
			true,
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"unfinished","summary":[]}}`,
			ending,
		)
		for _, event := range events {
			if event.Type == aikit.StreamEventReasoningSummary {
				t.Fatalf("synthetic completion on %s: %#v", ending, events)
			}
		}
	}
}

func TestResponsesReasoningStreamProviderOption(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var wire map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Error(err)
		}
		for _, key := range []string{"emitReasoningEvents", "emit_reasoning_events"} {
			if _, ok := wire[key]; ok {
				t.Errorf("local stream option leaked to provider: %s", key)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(
			w,
			"data: {\"type\":\"response.reasoning_summary_text.delta\",\"item_id\":\"rs_1\",\"delta\":\"x\"}\n\n"+
				"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"reasoning\",\"id\":\"rs_1\",\"summary\":[{\"type\":\"summary_text\",\"text\":\" x \"}]}}\n\n",
		)
	}))
	defer server.Close()
	model := NewLanguageModel("example-model", Config{BaseURL: server.URL})
	for _, options := range []any{ProviderOptions{AllowMissingTerminalEvent: true}, ProviderOptions{EmitReasoningEvents: true, AllowMissingTerminalEvent: true}, map[string]any{"emitReasoningEvents": true, "allowMissingTerminalEvent": true}} {
		stream, err := model.Stream(
			context.Background(),
			llm.Request{ProviderOptions: map[string]any{"openai": options}},
		)
		if err != nil {
			t.Fatal(err)
		}
		var events []aikit.StreamEvent
		for event := range stream {
			if event.Type == aikit.StreamEventError {
				t.Fatal(event.Error)
			}
			events = append(events, event)
		}
		enabled := true
		if typed, ok := options.(ProviderOptions); ok {
			enabled = typed.EmitReasoningEvents
		}
		if !enabled {
			if len(events) != 1 || events[0].ReasoningID != nil {
				t.Fatalf("default: %#v", events)
			}
		} else if len(events) != 2 || events[0].ReasoningID == nil || events[1].Type != aikit.StreamEventReasoningSummary || events[1].ReasoningSummary != "x" {
			t.Fatalf("enabled: %#v", events)
		}
	}
}

func TestResponsesReasoningAddedSnapshotsAndPartSeeds(t *testing.T) {
	events := reasoningStreamEvents(
		t,
		true,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":" First "},{"type":"encrypted","text":"hidden"}]}}`,
		`{"type":"response.reasoning_summary_part.added","output_index":0,"item_id":"rs_1","summary_index":2,"part":{"type":"summary_text","text":" Second"}}`,
		`{"type":"response.reasoning_summary_text.delta","output_index":0,"item_id":"rs_1","summary_index":2,"delta":" part "}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1"}}`,
	)
	if len(events) != 2 || events[1].ReasoningSummary != "First\n\nSecond part" {
		t.Fatalf("added snapshots lost source summaries: %#v", events)
	}
}
