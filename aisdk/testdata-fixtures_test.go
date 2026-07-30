package aisdk

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFixture_TextOnly verifies the text-only fixture is well-formed and contains required chunks.
func TestFixture_TextOnly(t *testing.T) {
	lines := readFixtureLines(t, "testdata/text-only.jsonl")
	assertAnyLineContains(t, lines, `"type":"start"`)
	assertAnyLineContains(t, lines, `"type":"start-step"`)
	assertAnyLineContains(t, lines, `"type":"text-start"`)
	assertAnyLineContains(t, lines, `"type":"text-delta"`)
	assertAnyLineContains(t, lines, `"type":"text-end"`)
	assertAnyLineContains(t, lines, `"type":"finish-step"`)
	assertAnyLineContains(t, lines, `"type":"finish"`)
	assertAnyLineContains(t, lines, "[DONE]")
}

// TestFixture_ReasoningWithSources verifies reasoning + source chunks are present.
func TestFixture_ReasoningWithSources(t *testing.T) {
	lines := readFixtureLines(t, "testdata/reasoning-with-sources.jsonl")
	assertAnyLineContains(t, lines, `"type":"start"`)
	assertAnyLineContains(t, lines, `"type":"reasoning-start"`)
	assertAnyLineContains(t, lines, `"type":"reasoning-delta"`)
	assertAnyLineContains(t, lines, `"type":"reasoning-end"`)
	assertAnyLineContains(t, lines, `"type":"text-start"`)
	assertAnyLineContains(t, lines, `"type":"text-delta"`)
	// One source-url chunk per reference. These fixtures previously carried
	// type "source" with an "id" field, plus a batch "sources" chunk. None of the
	// three exists in v7, so the recorded streams were non-conformant.
	assertAnyLineContains(t, lines, `"type":"source-url"`)
	assertAnyLineContains(t, lines, `"sourceId":`)
	assertNoneContains(t, lines, `"type":"sources"`)
	assertAnyLineContains(t, lines, `"type":"finish"`)
	assertAnyLineContains(t, lines, "[DONE]")
}

// TestFixture_ToolCallLifecycle verifies the full tool call sequence including custom data chunks.
func TestFixture_ToolCallLifecycle(t *testing.T) {
	lines := readFixtureLines(t, "testdata/tool-call-lifecycle.jsonl")
	assertAnyLineContains(t, lines, `"type":"start"`)
	assertAnyLineContains(t, lines, `"type":"tool-input-start"`)
	assertAnyLineContains(t, lines, `"type":"tool-input-delta"`)
	assertAnyLineContains(t, lines, `"type":"tool-input-available"`)
	assertAnyLineContains(t, lines, `"type":"tool-output-available"`)
	assertAnyLineContains(t, lines, `"type":"data-document-references"`)
	assertAnyLineContains(t, lines, `"type":"data-usage"`)
	assertAnyLineContains(t, lines, `"type":"finish"`)
	assertAnyLineContains(t, lines, "[DONE]")
}

// TestFixture_DeepThinkingFull verifies the deep-thinking scenario covers all planned chunk types.
func TestFixture_DeepThinkingFull(t *testing.T) {
	lines := readFixtureLines(t, "testdata/deep-thinking-full.jsonl")
	assertAnyLineContains(t, lines, `"type":"start"`)
	assertAnyLineContains(t, lines, `"type":"data-plan"`)
	assertAnyLineContains(t, lines, `"type":"data-steps"`)
	assertAnyLineContains(t, lines, `"type":"reasoning-start"`)
	assertAnyLineContains(t, lines, `"type":"reasoning-delta"`)
	assertAnyLineContains(t, lines, `"type":"reasoning-end"`)
	assertAnyLineContains(t, lines, `"type":"tool-input-start"`)
	assertAnyLineContains(t, lines, `"type":"tool-output-available"`)
	assertAnyLineContains(t, lines, `"type":"text-start"`)
	assertAnyLineContains(t, lines, `"type":"text-delta"`)
	// One source-url chunk per reference. These fixtures previously carried
	// type "source" with an "id" field, plus a batch "sources" chunk. None of the
	// three exists in v7, so the recorded streams were non-conformant.
	assertAnyLineContains(t, lines, `"type":"source-url"`)
	assertAnyLineContains(t, lines, `"sourceId":`)
	assertNoneContains(t, lines, `"type":"sources"`)
	assertAnyLineContains(t, lines, `"type":"data-suggested-questions"`)
	assertAnyLineContains(t, lines, `"type":"data-usage"`)
	assertAnyLineContains(t, lines, `"type":"finish"`)
	assertAnyLineContains(t, lines, "[DONE]")
}

// TestFixture_ErrorMidStream verifies the error fixture terminates with an error chunk and no finish.
func TestFixture_ErrorMidStream(t *testing.T) {
	lines := readFixtureLines(t, "testdata/error-mid-stream.jsonl")
	assertAnyLineContains(t, lines, `"type":"start"`)
	assertAnyLineContains(t, lines, `"type":"text-delta"`)
	assertAnyLineContains(t, lines, `"type":"error"`)
	assertNoneContains(t, lines, `"type":"finish"`)
	assertNoneContains(t, lines, "[DONE]")
}

// TestFixtures_EveryChunkTypeIsInTheProtocolUnion is the guard that was missing.
//
// Each fixture above asserts the types it expects to find, which cannot catch a type
// that should not be there at all — that is how "source" and "sources" survived in two
// recorded streams. This walks every frame of every fixture and rejects any type
// outside the client's union, so a non-conformant chunk cannot hide in testdata.
func TestFixtures_EveryChunkTypeIsInTheProtocolUnion(t *testing.T) {
	known := map[string]bool{
		ChunkStart: true, ChunkStartStep: true, ChunkFinishStep: true, ChunkFinish: true,
		ChunkTextStart: true, ChunkTextDelta: true, ChunkTextEnd: true,
		ChunkReasoningStart: true, ChunkReasoningDelta: true, ChunkReasoningEnd: true,
		ChunkToolInputStart: true, ChunkToolInputDelta: true, ChunkToolInputAvailable: true,
		ChunkToolInputError: true, ChunkToolOutputAvailable: true, ChunkToolOutputError: true,
		ChunkToolOutputDenied: true, ChunkToolApprovalRequest: true,
		ChunkToolApprovalResponse: true, ChunkSourceURL: true, ChunkSourceDocument: true,
		ChunkFile: true, ChunkReasoningFile: true, ChunkCustom: true,
		ChunkAbort: true, ChunkMessageMetadata: true, ChunkError: true,
	}

	fixtures, err := filepath.Glob("testdata/*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no fixtures found")
	}

	for _, path := range fixtures {
		for i, line := range readFixtureLines(t, path) {
			payload, ok := strings.CutPrefix(line, "data: ")
			if !ok {
				t.Errorf("%s line %d is not an SSE data line: %s", path, i+1, line)
				continue
			}
			if payload == "[DONE]" {
				continue
			}
			var frame struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal([]byte(payload), &frame); err != nil {
				t.Errorf("%s line %d: %v", path, i+1, err)
				continue
			}
			// data-${name} is an open family, matched by prefix.
			if strings.HasPrefix(frame.Type, "data-") {
				continue
			}
			if !known[frame.Type] {
				t.Errorf("%s line %d: chunk type %q is not in the v7 union",
					path, i+1, frame.Type)
			}
		}
	}
}

// --- helpers ---

func readFixtureLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture %s: %v", path, err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan fixture %s: %v", path, err)
	}
	return lines
}

func assertAnyLineContains(t *testing.T, lines []string, want string) {
	t.Helper()
	for _, l := range lines {
		if strings.Contains(l, want) {
			return
		}
	}
	t.Errorf("no fixture line contains %q", want)
}

func assertNoneContains(t *testing.T, lines []string, want string) {
	t.Helper()
	for _, l := range lines {
		if strings.Contains(l, want) {
			t.Errorf("fixture line contains %q but should not: %s", want, l)
			return
		}
	}
}
