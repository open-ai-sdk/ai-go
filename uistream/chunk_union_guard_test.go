package uistream

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These literals mirror ui-message-chunks.ts in ai@7.0.35. Phase 01 replaces
// this temporary copy with a list generated from the installed package.
var uiMessageChunkTypes = map[string]struct{}{
	ChunkTextStart:            {},
	ChunkTextDelta:            {},
	ChunkTextEnd:              {},
	ChunkError:                {},
	ChunkToolInputStart:       {},
	ChunkToolInputDelta:       {},
	ChunkToolInputAvailable:   {},
	ChunkToolInputError:       {},
	ChunkToolApprovalRequest:  {},
	ChunkToolApprovalResponse: {},
	ChunkToolOutputAvailable:  {},
	ChunkToolOutputError:      {},
	ChunkToolOutputDenied:     {},
	ChunkReasoningStart:       {},
	ChunkReasoningDelta:       {},
	ChunkReasoningEnd:         {},
	ChunkCustom:               {},
	ChunkSourceURL:            {},
	ChunkSourceDocument:       {},
	ChunkFile:                 {},
	ChunkReasoningFile:        {},
	ChunkStartStep:            {},
	ChunkFinishStep:           {},
	ChunkStart:                {},
	ChunkFinish:               {},
	ChunkAbort:                {},
	ChunkMessageMetadata:      {},
}

func isUIMessageChunkType(typ string) bool {
	if strings.HasPrefix(typ, "data-") {
		return true
	}
	_, ok := uiMessageChunkTypes[typ]
	return ok
}

func TestUIMessageChunkTypeGuard(t *testing.T) {
	for typ := range uiMessageChunkTypes {
		if !isUIMessageChunkType(typ) {
			t.Errorf("known UI message chunk type %q was rejected", typ)
		}
	}

	for _, typ := range []string{
		"data-plan",
		"data-steps",
		"data-document-references",
		"data-usage",
		"data-suggested-questions",
		"data-structured-output",
	} {
		if !isUIMessageChunkType(typ) {
			t.Errorf("custom data chunk type %q was rejected", typ)
		}
	}

	for _, typ := range []string{"source", "sources", "unknown"} {
		if isUIMessageChunkType(typ) {
			t.Errorf("non-protocol chunk type %q was accepted", typ)
		}
	}
}

func TestFixturesContainOnlyUIMessageChunkTypes(t *testing.T) {
	fixtures, err := filepath.Glob("testdata/*.jsonl")
	if err != nil {
		t.Fatalf("find fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no UI message stream fixtures found")
	}

	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			f, err := os.Open(fixture)
			if err != nil {
				t.Fatalf("open fixture: %v", err)
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			for line := 1; scanner.Scan(); line++ {
				payload := strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "data: ")
				if payload == "" || payload == "[DONE]" {
					continue
				}
				var frame struct {
					Type string `json:"type"`
				}
				if err := json.Unmarshal([]byte(payload), &frame); err != nil {
					t.Fatalf("line %d: decode frame: %v", line, err)
				}
				if !isUIMessageChunkType(frame.Type) {
					t.Errorf("line %d: non-protocol chunk type %q", line, frame.Type)
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scan fixture: %v", err)
			}
		})
	}
}
