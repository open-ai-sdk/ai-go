package aikit

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRichToolResultPartClonePreservesAndOwnsContent(t *testing.T) {
	source := RichToolResultPart("call_1", "lookup",
		TextToolResultContent("literal text"),
		JSONToolResultContent(json.RawMessage(`{"answer":42}`)),
		ImageToolResultContent([]byte{1, 2, 3}, "image/png"),
		ImageURLToolResultContent("https://example.test/image.png", "image/png"),
	)
	clone := source.Clone()
	if !reflect.DeepEqual(clone, source) {
		t.Fatalf("clone lost rich content: %#v != %#v", clone, source)
	}
	clone.ToolResultContent[0].Text = "changed"
	clone.ToolResultContent[1].JSON[2] = 'X'
	clone.ToolResultContent[2].Data[0] = 9
	clone.ToolResultContent[3].FileURL = "https://example.test/changed.png"
	if source.ToolResultContent[0].Text != "literal text" ||
		string(source.ToolResultContent[1].JSON) != `{"answer":42}` ||
		source.ToolResultContent[2].Data[0] != 1 ||
		source.ToolResultContent[3].FileURL != "https://example.test/image.png" {
		t.Fatalf("clone shares mutable content: %#v", source)
	}
}

func TestImageURLToolResultContentDoesNotInventInlineData(t *testing.T) {
	part := ImageURLToolResultContent("https://example.test/image.png", "")
	if part.Type != ToolResultContentTypeImage || part.FileURL != "https://example.test/image.png" ||
		part.Data != nil ||
		part.MediaType != "" {
		t.Fatalf("unexpected URL reference: %#v", part)
	}
}
