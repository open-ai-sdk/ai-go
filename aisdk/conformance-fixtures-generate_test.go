package aisdk

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The conformance fixtures are generated here, by the same constructors the producer
// uses, and consumed by ai-go/conformance/ — a Vitest suite that runs the real
// ai@7.0.35 client over them.
//
//	go test ./aisdk/ -run TestConformanceFixtures -update-fixtures
//
// The .sse files are generated; the .expected.json files are hand-written from reading
// the TypeScript processor. That split is deliberate: if both sides were generated from
// Go, the harness would assert only that Go agrees with itself.
var updateFixtures = flag.Bool("update-fixtures", false, "regenerate ../conformance/fixtures")

const fixtureDir = "../conformance/fixtures"

// conformanceFixture is one named SSE stream.
//
// invalid fixtures are expected to make the client's stream processor throw (layer 2);
// invalidSchema fixtures are expected to fail the chunk schema itself (layer 1). They
// live in separate directories because they fail at different layers, and a test that
// cannot say which layer caught it is not much of a test.
type conformanceFixture struct {
	name   string
	dir    string // "", "invalid", or "invalid-schema"
	chunks []Chunk
}

func conformanceFixtures() []conformanceFixture {
	// A leaky provider error, so every fixture carrying error text also exercises
	// redaction rather than asserting it separately.
	provErr := &APIError{StatusCode: 500, Message: "org_id=acme request_id=req_1 secret"}

	return []conformanceFixture{
		{name: "text-simple", chunks: []Chunk{
			StartChunk("msg-text-simple"),
			StartStep(),
			TextStart("t0"),
			TextDeltaChunk("t0", "Hello "),
			TextDeltaChunk("t0", "world"),
			TextEnd("t0"),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		// Two text segments split by a tool call. The second segment MUST use a new
		// block id: reusing t0 after finish-step would have the client either
		// overwrite the earlier part or push a duplicate, and it never throws for it.
		{name: "text-multi-segment-across-tool", chunks: []Chunk{
			StartChunk("msg-multi-seg"),
			StartStep(),
			TextStart("t0"),
			TextDeltaChunk("t0", "Let me check."),
			TextEnd("t0"),
			ToolInputStart("call_1", "getWeather"),
			ToolInputDelta("call_1", `{"city":`),
			ToolInputDelta("call_1", `"Hanoi"}`),
			ToolInputAvailable("call_1", "getWeather", map[string]any{"city": "Hanoi"}),
			ToolOutputAvailable("call_1", map[string]any{"tempC": 31}),
			FinishStep(),
			StartStep(),
			TextStart("t1"),
			TextDeltaChunk("t1", "It is 31C."),
			TextEnd("t1"),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		// The signature rides on reasoning-end.providerMetadata. Emitting it as a
		// reasoning-delta with empty text would both lose it and produce a useless
		// empty delta.
		{name: "reasoning-with-signature", chunks: []Chunk{
			StartChunk("msg-reasoning"),
			StartStep(),
			ReasoningStart("r0"),
			ReasoningDeltaChunk("r0", "Thinking about it. "),
			ReasoningDeltaChunk("r0", "Done thinking."),
			ReasoningEnd("r0", WithProviderMetadata(map[string]any{
				"anthropic": map[string]any{"signature": "sig-abc123"},
			})),
			TextStart("t0"),
			TextDeltaChunk("t0", "Answer."),
			TextEnd("t0"),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		{name: "tool-server-executed", chunks: []Chunk{
			StartChunk("msg-tool-server"),
			StartStep(),
			ToolInputStart("call_1", "getWeather", WithProviderExecuted(false)),
			ToolInputDelta("call_1", `{"city":"Hanoi"}`),
			ToolInputAvailable("call_1", "getWeather", map[string]any{"city": "Hanoi"},
				WithProviderExecuted(false)),
			ToolOutputAvailable("call_1", map[string]any{"tempC": 31},
				WithProviderExecuted(false)),
			FinishStep(),
			FinishChunk(WireFinishToolCalls),
		}},

		// A provider-executed tool has no input deltas: its arguments arrive whole,
		// not as partial JSON.
		{name: "tool-provider-executed", chunks: []Chunk{
			StartChunk("msg-tool-provider"),
			StartStep(),
			ToolInputAvailable("call_2", "web_search", map[string]any{"q": "hanoi"},
				WithProviderExecuted(true)),
			ToolOutputAvailable("call_2", "3 results", WithProviderExecuted(true)),
			FinishStep(),
			FinishChunk(WireFinishToolCalls),
		}},

		// Turn 1 of a gated tool: the request goes out and the turn ends. Nothing is
		// executed, so there is no output chunk.
		{name: "tool-approval-request", chunks: []Chunk{
			StartChunk("msg-approval-req"),
			StartStep(),
			ToolInputAvailable("call_3", "sendEmail", map[string]any{"to": "a@b.c"}),
			ToolApprovalRequest("appr_1", "call_3", WithApprovalSignature("sig-approval-1")),
			FinishStep(),
			FinishChunk(WireFinishToolCalls),
		}},

		// Turn 2, approved: the same toolCallId gets its output. No second
		// tool-input-start — that would open a duplicate part.
		{name: "tool-approval-approved", chunks: []Chunk{
			StartChunk("msg-approval-ok"),
			StartStep(),
			ToolInputAvailable("call_3", "sendEmail", map[string]any{"to": "a@b.c"}),
			ToolApprovalRequest("appr_1", "call_3"),
			ToolApprovalResponseChunk("appr_1", true),
			ToolOutputAvailable("call_3", "sent"),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		// Turn 2, denied: tool-output-denied, never an error chunk. A denial is a
		// user decision, not a failure.
		{name: "tool-approval-denied", chunks: []Chunk{
			StartChunk("msg-approval-no"),
			StartStep(),
			ToolInputAvailable("call_3", "sendEmail", map[string]any{"to": "a@b.c"}),
			ToolApprovalRequest("appr_1", "call_3"),
			ToolApprovalResponseChunk("appr_1", false, WithApprovalReason("user declined")),
			ToolOutputDenied("call_3"),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		{name: "tool-input-error", chunks: []Chunk{
			StartChunk("msg-tool-input-err"),
			StartStep(),
			ToolInputStart("call_4", "getWeather"),
			ToolInputDelta("call_4", `{"city":`),
			ToolInputError("call_4", "getWeather", `{"city":`, provErr),
			FinishStep(),
			FinishChunk(WireFinishError),
		}},

		{name: "tool-output-error", chunks: []Chunk{
			StartChunk("msg-tool-output-err"),
			StartStep(),
			ToolInputAvailable("call_5", "getWeather", map[string]any{"city": "Nowhere"}),
			ToolOutputError("call_5", provErr),
			FinishStep(),
			FinishChunk(WireFinishError),
		}},

		// A stream-level failure. The client calls onError and breaks — it does not
		// throw and does not unwind the text already rendered.
		{name: "error", chunks: []Chunk{
			StartChunk("msg-error"),
			StartStep(),
			TextStart("t0"),
			TextDeltaChunk("t0", "Partial answer"),
			ErrorChunk(provErr),
		}},

		// Source chunks ARE produced: chunk-types-and-producer.go emits source-url from
		// StepEventSource, and Writer has WriteSourceURL / WriteSourceDocument. An
		// earlier reading treated the whole source family as unproducible and left it
		// out, which meant four live emission paths had no client-validated coverage —
		// exactly where the "source"/"sourceId" bug had been hiding.
		{name: "source-url-and-document", chunks: []Chunk{
			StartChunk("msg-sources"),
			StartStep(),
			TextStart("t0"),
			TextDeltaChunk("t0", "Per two sources."),
			TextEnd("t0"),
			SourceURL("src_1", "https://example.com/a", WithSourceTitle("Article A")),
			SourceURL("src_2", "https://example.com/b"), // title omitted, not ""
			SourceDocument("doc_1", "application/pdf", "Spec v3",
				WithFilename("spec-v3.pdf")),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		{name: "file-and-reasoning-file", chunks: []Chunk{
			StartChunk("msg-files"),
			StartStep(),
			FileChunk("data:image/png;base64,iVBORw0KGgo=", "image/png"),
			ReasoningFileChunk("data:text/plain;base64,dGhvdWdodHM=", "text/plain"),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		// --- negative: layer 2, the client's stream processor must throw ---

		{name: "text-delta-without-start", dir: "invalid", chunks: []Chunk{
			StartChunk("msg-bad-text"),
			StartStep(),
			TextDeltaChunk("t0", "orphan"),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		{name: "reasoning-delta-without-start", dir: "invalid", chunks: []Chunk{
			StartChunk("msg-bad-reasoning"),
			StartStep(),
			ReasoningDeltaChunk("r0", "orphan"),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		{name: "tool-input-delta-without-start", dir: "invalid", chunks: []Chunk{
			StartChunk("msg-bad-tool-input"),
			StartStep(),
			ToolInputDelta("call_1", `{"a":1}`),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		// An approval response naming an approvalId nobody requested.
		{name: "unknown-approval-id", dir: "invalid", chunks: []Chunk{
			StartChunk("msg-bad-approval"),
			StartStep(),
			ToolInputAvailable("call_3", "sendEmail", map[string]any{"to": "a@b.c"}),
			ToolApprovalResponseChunk("appr_does_not_exist", true),
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},

		// --- negative: layer 1, the chunk schema itself must reject ---
		//
		// Built as a raw Chunk on purpose: no constructor can produce an unknown type,
		// which is the point — layer 1 exists to catch what the Go API prevents.
		{name: "unknown-chunk-type", dir: "invalid-schema", chunks: []Chunk{
			StartChunk("msg-unknown-type"),
			StartStep(),
			{Type: "text-suffix", Fields: map[string]any{"id": "t0", "suffix": "!"}},
			FinishStep(),
			FinishChunk(WireFinishStop),
		}},
	}
}

// renderFixture frames a fixture exactly as the wire carries it.
func renderFixture(t *testing.T, chunks []Chunk) string {
	t.Helper()
	var sb strings.Builder
	sw := NewSSEWriter(&sb)
	for _, c := range chunks {
		if err := sw.WriteChunk(c); err != nil {
			t.Fatalf("WriteChunk %s: %v", c.Type, err)
		}
	}
	if err := sw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return sb.String()
}

func fixturePath(f conformanceFixture) string {
	if f.dir == "" {
		return filepath.Join(fixtureDir, f.name+".sse")
	}
	return filepath.Join(fixtureDir, f.dir, f.name+".sse")
}

// TestConformanceFixtures asserts the committed fixtures match what the constructors
// produce today, so a chunk change shows up as a diff here and then as a TS failure in
// the conformance suite.
func TestConformanceFixtures(t *testing.T) {
	for _, f := range conformanceFixtures() {
		path := fixturePath(f)
		got := renderFixture(t, f.chunks)

		if *updateFixtures {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}

		want, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v (regenerate with -update-fixtures)", path, err)
			continue
		}
		if got != string(want) {
			t.Errorf("%s drift:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
		}
	}
	if *updateFixtures {
		t.Logf("regenerated %d fixtures under %s — now run: cd conformance && pnpm test",
			len(conformanceFixtures()), fixtureDir)
	}
}

// TestConformanceFixtures_NamesAreUnique guards against one fixture silently
// overwriting another, which would look like coverage while removing it.
func TestConformanceFixtures_NamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range conformanceFixtures() {
		key := f.dir + "/" + f.name
		if seen[key] {
			t.Errorf("duplicate fixture %q", key)
		}
		seen[key] = true
	}
}

// TestConformanceFixtures_NegativesAreActuallyMalformed checks that each negative
// fixture contains the defect it is named for. A negative fixture that is accidentally
// valid passes the TS suite for the wrong reason, which is worse than not having it.
func TestConformanceFixtures_NegativesAreActuallyMalformed(t *testing.T) {
	byName := map[string]conformanceFixture{}
	for _, f := range conformanceFixtures() {
		if f.dir != "" {
			byName[f.name] = f
		}
	}

	// Each negative must contain the delta/response chunk and must NOT contain the
	// -start chunk that would make it valid.
	cases := []struct {
		fixture     string
		mustContain string
		mustNotHave string
	}{
		{"text-delta-without-start", ChunkTextDelta, ChunkTextStart},
		{"reasoning-delta-without-start", ChunkReasoningDelta, ChunkReasoningStart},
		{"tool-input-delta-without-start", ChunkToolInputDelta, ChunkToolInputStart},
		{"unknown-approval-id", ChunkToolApprovalResponse, ChunkToolApprovalRequest},
	}
	for _, tc := range cases {
		f, ok := byName[tc.fixture]
		if !ok {
			t.Errorf("negative fixture %q is missing", tc.fixture)
			continue
		}
		var hasBad, hasGood bool
		for _, c := range f.chunks {
			if c.Type == tc.mustContain {
				hasBad = true
			}
			if c.Type == tc.mustNotHave {
				hasGood = true
			}
		}
		if !hasBad {
			t.Errorf("%s: does not contain a %s chunk", tc.fixture, tc.mustContain)
		}
		if hasGood {
			t.Errorf("%s: contains %s, so it is actually valid and cannot fail",
				tc.fixture, tc.mustNotHave)
		}
	}

	// The layer-1 negative must carry a type no constructor can emit.
	f, ok := byName["unknown-chunk-type"]
	if !ok {
		t.Fatal("unknown-chunk-type fixture is missing")
	}
	if f.dir != "invalid-schema" {
		t.Errorf("unknown-chunk-type is in %q, want invalid-schema — it fails at the "+
			"schema layer, not the processor layer", f.dir)
	}
	var hasUnknown bool
	for _, c := range f.chunks {
		if c.Type == "text-suffix" {
			hasUnknown = true
		}
	}
	if !hasUnknown {
		t.Error("unknown-chunk-type no longer contains an unknown chunk type")
	}
}
