package aisdk

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type uiMessageChunkUnion struct {
	LiteralTypes []string `json:"literalTypes"`
	PrefixTypes  []string `json:"prefixTypes"`
}

func loadUIMessageChunkUnion(t *testing.T) uiMessageChunkUnion {
	t.Helper()
	path := filepath.Join("..", "conformance", "src", "ui_message_chunk_types.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated UI message chunk union %s: %v", path, err)
	}
	var union uiMessageChunkUnion
	if err := json.Unmarshal(data, &union); err != nil {
		t.Fatalf("decode generated UI message chunk union: %v", err)
	}
	if len(union.LiteralTypes) != 27 {
		t.Fatalf("generated union has %d literal types, want 27", len(union.LiteralTypes))
	}
	if len(union.PrefixTypes) != 1 || union.PrefixTypes[0] != "data-" {
		t.Fatalf("generated union prefix types = %v, want [data-]", union.PrefixTypes)
	}
	return union
}

func (union uiMessageChunkUnion) contains(typ string) bool {
	for _, prefix := range union.PrefixTypes {
		if strings.HasPrefix(typ, prefix) {
			return true
		}
	}
	return slices.Contains(union.LiteralTypes, typ)
}

func TestUIMessageChunkTypeGuard(t *testing.T) {
	union := loadUIMessageChunkUnion(t)
	for _, typ := range union.LiteralTypes {
		if !union.contains(typ) {
			t.Errorf("known UI message chunk type %q was rejected", typ)
		}
		if !ValidChunkType(typ) {
			t.Errorf("production union guard rejected %q", typ)
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
		if !union.contains(typ) {
			t.Errorf("custom data chunk type %q was rejected", typ)
		}
		if !ValidChunkType(typ) {
			t.Errorf("production union guard rejected %q", typ)
		}
	}

	for _, typ := range []string{"source", "sources", "unknown"} {
		if union.contains(typ) {
			t.Errorf("non-protocol chunk type %q was accepted", typ)
		}
		if ValidChunkType(typ) {
			t.Errorf("production union guard accepted %q", typ)
		}
	}
}

func TestFixturesContainOnlyUIMessageChunkTypes(t *testing.T) {
	union := loadUIMessageChunkUnion(t)
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
				if !union.contains(frame.Type) {
					t.Errorf("line %d: non-protocol chunk type %q", line, frame.Type)
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scan fixture: %v", err)
			}
		})
	}
}
