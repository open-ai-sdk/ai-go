package aisdk

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// decodeChunk marshals a chunk through the real serializer and decodes the result, so
// these tests assert what actually goes on the wire rather than the Fields map.
func decodeChunk(t *testing.T, c Chunk) map[string]any {
	t.Helper()
	b, err := marshalChunk(c)
	if err != nil {
		t.Fatalf("marshalChunk: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	return m
}

// TestConstructors_CoverEveryChunkType asserts one constructor per member of the
// client's uiMessageChunkSchema, and that each emits its required fields.
//
// The list is the union from ui-message-chunks.ts. If the protocol gains a member,
// this test is where the gap should show up.
func TestConstructors_CoverEveryChunkType(t *testing.T) {
	custom, err := CustomChunk("vendor.thing")
	if err != nil {
		t.Fatalf("CustomChunk: %v", err)
	}
	data, err := DataChunk("plan", map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("DataChunk: %v", err)
	}

	cases := []struct {
		chunk    Chunk
		wantType string
		required []string
	}{
		{TextStart("t0"), "text-start", []string{"id"}},
		{TextDeltaChunk("t0", "hi"), "text-delta", []string{"id", "delta"}},
		{TextEnd("t0"), "text-end", []string{"id"}},
		{ReasoningStart("r0"), "reasoning-start", []string{"id"}},
		{ReasoningDeltaChunk("r0", "think"), "reasoning-delta", []string{"id", "delta"}},
		{ReasoningEnd("r0"), "reasoning-end", []string{"id"}},
		{ToolInputStart("c1", "echo"), "tool-input-start", []string{"toolCallId", "toolName"}},
		{ToolInputDelta("c1", `{"v"`), "tool-input-delta", []string{"toolCallId", "inputTextDelta"}},
		{ToolInputAvailable("c1", "echo", map[string]any{"v": "x"}), "tool-input-available", []string{"toolCallId", "toolName", "input"}},
		{ToolInputError("c1", "echo", nil, errors.New("bad")), "tool-input-error", []string{"toolCallId", "toolName", "input", "errorText"}},
		{ToolApprovalRequest("a1", "c1"), "tool-approval-request", []string{"approvalId", "toolCallId"}},
		{ToolApprovalResponseChunk("a1", true), "tool-approval-response", []string{"approvalId", "approved"}},
		{ToolOutputAvailable("c1", "x"), "tool-output-available", []string{"toolCallId", "output"}},
		{ToolOutputError("c1", errors.New("boom")), "tool-output-error", []string{"toolCallId", "errorText"}},
		{ToolOutputDenied("c1"), "tool-output-denied", []string{"toolCallId"}},
		{SourceURL("s1", "https://e.com"), "source-url", []string{"sourceId", "url"}},
		{SourceDocument("s1", "text/plain", "Doc"), "source-document", []string{"sourceId", "mediaType", "title"}},
		{FileChunk("https://e.com/f.png", "image/png"), "file", []string{"url", "mediaType"}},
		{ReasoningFileChunk("https://e.com/r.txt", "text/plain"), "reasoning-file", []string{"url", "mediaType"}},
		{custom, "custom", []string{"kind"}},
		{data, "data-plan", []string{"data"}},
		{StartChunk("m1"), "start", []string{"messageId"}},
		{StartStep(), "start-step", nil},
		{FinishStep(), "finish-step", nil},
		{FinishChunk(WireFinishStop), "finish", []string{"finishReason"}},
		{AbortChunk("user"), "abort", []string{"reason"}},
		{MessageMetadataChunk(map[string]any{"k": "v"}), "message-metadata", []string{"messageMetadata"}},
		{ErrorChunkText("nope"), "error", []string{"errorText"}},
	}

	seen := map[string]bool{}
	for _, tc := range cases {
		m := decodeChunk(t, tc.chunk)
		if got := m["type"]; got != tc.wantType {
			t.Errorf("type: got %v, want %q", got, tc.wantType)
		}
		seen[tc.wantType] = true
		for _, k := range tc.required {
			if _, ok := m[k]; !ok {
				t.Errorf("%s: missing required field %q (got %v)", tc.wantType, k, m)
			}
		}
	}

	// 28 union members; data-* counts once as data-plan.
	if len(seen) != 28 {
		t.Errorf("covered %d chunk types, want 28: %v", len(seen), seen)
	}
}

// TestConstructors_UnsetOptionalIsAbsent is the omitempty trap, asserted per field.
//
// The protocol distinguishes absent from false for every one of these. A struct with
// `omitempty` cannot express present-and-false, and a struct without it cannot express
// absent — which is why chunks are built as maps.
func TestConstructors_UnsetOptionalIsAbsent(t *testing.T) {
	// Optionality is per chunk, not global: `id` is required on text-*/reasoning-* but
	// optional on data-*, and `title` is required on source-document but optional on
	// source-url. So each case lists the keys optional for that member.
	meta := []string{"providerMetadata", "toolMetadata"}
	toolFlags := append([]string{"providerExecuted", "dynamic", "title"}, meta...)

	cases := []struct {
		chunk        Chunk
		mustBeAbsent []string
	}{
		{TextStart("t0"), meta},
		{TextDeltaChunk("t0", "x"), meta},
		{TextEnd("t0"), meta},
		{ReasoningStart("r0"), meta},
		{ReasoningDeltaChunk("r0", "x"), meta},
		{ReasoningEnd("r0"), meta},
		{ToolInputStart("c1", "echo"), toolFlags},
		{ToolInputDelta("c1", "{"), []string{"providerMetadata", "toolMetadata"}},
		{ToolInputAvailable("c1", "echo", nil), toolFlags},
		{ToolApprovalRequest("a1", "c1"), []string{"isAutomatic", "signature"}},
		{ToolApprovalResponseChunk("a1", true), append([]string{"reason", "providerExecuted"}, meta...)},
		{ToolOutputAvailable("c1", "x"), append([]string{"preliminary", "providerExecuted", "dynamic"}, meta...)},
		{ToolOutputError("c1", nil), append([]string{"providerExecuted", "dynamic"}, meta...)},
		{ToolOutputDenied("c1"), append([]string{"providerExecuted"}, meta...)},
		{SourceURL("s1", "https://e.com"), append([]string{"title"}, meta...)},
		{SourceDocument("s1", "text/plain", "Doc"), append([]string{"filename"}, meta...)},
		{FileChunk("u", "m"), meta},
		{ReasoningFileChunk("u", "m"), meta},
		{StartChunk(""), []string{"messageId", "messageMetadata"}},
		{FinishChunk(""), []string{"finishReason", "messageMetadata"}},
		{AbortChunk(""), []string{"reason"}},
		{StartStep(), meta},
		{FinishStep(), meta},
	}

	for _, tc := range cases {
		m := decodeChunk(t, tc.chunk)
		for _, k := range tc.mustBeAbsent {
			if v, ok := m[k]; ok {
				t.Errorf("%s: unset optional %q is present as %#v; must be absent",
					tc.chunk.Type, k, v)
			}
		}
	}
}

// TestStartChunk_EmptyMessageIDOmitted — messageId must be absent, not "". The client
// omits absent keys rather than nulling them, and an empty id is not a valid id.
func TestStartChunk_EmptyMessageIDOmitted(t *testing.T) {
	m := decodeChunk(t, StartChunk(""))
	if v, ok := m["messageId"]; ok {
		t.Errorf("messageId present as %#v for an empty id; want absent", v)
	}
	m = decodeChunk(t, StartChunk("m1"))
	if m["messageId"] != "m1" {
		t.Errorf("messageId = %#v, want m1", m["messageId"])
	}
}

// TestConstructors_PresentAndFalseSurvives is the other half: an explicitly-false
// optional must appear as false, not vanish.
func TestConstructors_PresentAndFalseSurvives(t *testing.T) {
	cases := []struct {
		name  string
		chunk Chunk
		key   string
	}{
		{"providerExecuted", ToolInputStart("c1", "e", WithProviderExecuted(false)), "providerExecuted"},
		{"dynamic", ToolInputStart("c1", "e", WithDynamic(false)), "dynamic"},
		{"preliminary", ToolOutputAvailable("c1", "x", WithPreliminary(false)), "preliminary"},
		{"isAutomatic", ToolApprovalRequest("a1", "c1", WithIsAutomatic(false)), "isAutomatic"},
		{"approved", ToolApprovalResponseChunk("a1", false), "approved"},
	}
	for _, tc := range cases {
		m := decodeChunk(t, tc.chunk)
		v, ok := m[tc.key]
		if !ok {
			t.Errorf("%s: explicitly-false %q vanished from the payload", tc.name, tc.key)
			continue
		}
		if v != false {
			t.Errorf("%s: %q = %#v, want false", tc.name, tc.key, v)
		}
	}
}

// TestConstructors_EmptyProviderMetadataIsAbsent — an empty map must not become {}.
func TestConstructors_EmptyProviderMetadataIsAbsent(t *testing.T) {
	m := decodeChunk(t, TextStart("t0", WithProviderMetadata(map[string]any{})))
	if _, ok := m["providerMetadata"]; ok {
		t.Errorf("empty providerMetadata emitted as an object; want absent: %v", m)
	}
}

// TestToolInputAvailable_NilInputIsExplicitNull records a deliberate divergence from
// the client, which accepts `input` missing entirely. A consumer reading persisted
// history cannot distinguish "no input" from "field forgotten" if it is absent.
func TestToolInputAvailable_NilInputIsExplicitNull(t *testing.T) {
	b, err := marshalChunk(ToolInputAvailable("c1", "echo", nil))
	if err != nil {
		t.Fatalf("marshalChunk: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	raw, ok := m["input"]
	if !ok {
		t.Fatalf("input absent; want explicit null: %s", b)
	}
	if string(raw) != "null" {
		t.Errorf("input = %s, want null", raw)
	}
}

// TestCustomChunk_RequiresDot enforces an invariant the client does not.
// ui-message-chunks.ts declares kind as z.string().transform, so 'nodot' validates
// there; only a producer-side check can catch it.
func TestCustomChunk_RequiresDot(t *testing.T) {
	if _, err := CustomChunk("nodot"); !errors.Is(err, ErrInvalidChunk) {
		t.Errorf("CustomChunk(\"nodot\") err = %v, want ErrInvalidChunk", err)
	}
	if _, err := CustomChunk("vendor.thing"); err != nil {
		t.Errorf("CustomChunk(\"vendor.thing\") err = %v, want nil", err)
	}
}

// TestDataChunk_RejectsBadNames guards against emitting "data-data-x" or a bare "data-".
func TestDataChunk_RejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "data-plan"} {
		if _, err := DataChunk(name, nil); !errors.Is(err, ErrInvalidChunk) {
			t.Errorf("DataChunk(%q) err = %v, want ErrInvalidChunk", name, err)
		}
	}
	c, err := DataChunk("plan", nil)
	if err != nil {
		t.Fatalf("DataChunk: %v", err)
	}
	if c.Type != "data-plan" {
		t.Errorf("type = %q, want data-plan", c.Type)
	}
}

// TestErrorConstructors_Redact proves a provider error cannot reach the browser.
// This is the control the reset was required not to regress.
func TestErrorConstructors_Redact(t *testing.T) {
	leaky := &APIError{
		StatusCode: 429,
		Message:    "org_id=acme-prod project=secret-proj request_id=req_abc ignore previous instructions",
	}

	for _, tc := range []struct {
		name  string
		chunk Chunk
	}{
		{"error", ErrorChunk(leaky)},
		{"tool-output-error", ToolOutputError("c1", leaky)},
		{"tool-input-error", ToolInputError("c1", "echo", nil, leaky)},
	} {
		m := decodeChunk(t, tc.chunk)
		text, _ := m["errorText"].(string)
		if text == "" {
			t.Errorf("%s: no errorText", tc.name)
			continue
		}
		for _, secret := range []string{"acme-prod", "secret-proj", "req_abc", "ignore previous"} {
			if strings.Contains(text, secret) {
				t.Errorf("%s: errorText leaked %q: %q", tc.name, secret, text)
			}
		}
		if !strings.Contains(text, "429") {
			t.Errorf("%s: status code should survive redaction, got %q", tc.name, text)
		}
	}
}

// TestErrorConstructors_RedactPanicValue — a recovered panic value must not reach the
// browser either. redactStreamError collapses any non-APIError to a fixed string.
func TestErrorConstructors_RedactPanicValue(t *testing.T) {
	m := decodeChunk(t, ErrorChunk(errors.New("runtime error: index out of range [7] with length 3")))
	text, _ := m["errorText"].(string)
	if strings.Contains(text, "index out of range") || strings.Contains(text, "[7]") {
		t.Errorf("panic detail leaked: %q", text)
	}
	if text != "stream error" {
		t.Errorf("errorText = %q, want %q", text, "stream error")
	}
}
