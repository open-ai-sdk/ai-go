package openaicompat

import (
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

func encodeUserParts(t *testing.T, parts ...ai.ContentPart) (map[string]any, []ai.Warning) {
	t.Helper()
	req := ai.LanguageModelRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: parts}},
	}
	cr, warnings, err := EncodeRequest(EncodeRequestParams{ModelID: "test"}, req, true)
	if err != nil {
		t.Fatalf("EncodeRequest failed: %v", err)
	}
	if len(cr.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(cr.Messages))
	}
	return cr.Messages[0], warnings
}

func contentParts(t *testing.T, msg map[string]any) []map[string]any {
	t.Helper()
	parts, ok := msg["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected multipart content, got %#v", msg["content"])
	}
	return parts
}

// An image URL must be encoded as an image_url part. Before media-type routing
// replaced the old type discriminator, an ImageURLPart fell through to the
// generic-file branch and the image was replaced by a text placeholder.
func TestEncodeContentMessage_ImageURLBecomesImageURLPart(t *testing.T) {
	msg, warnings := encodeUserParts(t,
		ai.TextPart("what is this?"),
		ai.ImageURLPart("https://example.com/img.png"),
	)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	parts := contentParts(t, msg)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[1]["type"] != "image_url" {
		t.Fatalf("expected type=image_url, got %v", parts[1]["type"])
	}
	url := parts[1]["image_url"].(map[string]string)["url"]
	if url != "https://example.com/img.png" {
		t.Errorf("unexpected url: %q", url)
	}
}

// A bare "image" segment carries no subtype, so inline data falls back to PNG
// rather than producing a malformed "data:image;base64," URI.
func TestEncodeContentMessage_BareImageSegmentGetsPNGDataURI(t *testing.T) {
	msg, _ := encodeUserParts(t, ai.ImageDataPart([]byte("bytes"), "image"))
	parts := contentParts(t, msg)
	url := parts[0]["image_url"].(map[string]string)["url"]
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("expected a png data URI, got %q", url)
	}
}

// Chat completions has no file-reference form, so a file ID must warn instead of
// emitting an empty URL that the API rejects with an opaque error.
func TestEncodeContentMessage_FileIDWarnsInsteadOfEmptyURL(t *testing.T) {
	msg, warnings := encodeUserParts(t,
		ai.TextPart("describe"),
		ai.ImageFileIDPart("file-abc123"),
	)
	if len(warnings) != 1 || warnings[0].Setting != "fileID" {
		t.Fatalf("expected one fileID warning, got %+v", warnings)
	}
	parts := contentParts(t, msg)
	if len(parts) != 1 || parts[0]["type"] != "text" {
		t.Fatalf("expected only the text part to survive, got %+v", parts)
	}
	for _, p := range parts {
		if p["type"] == "image_url" {
			t.Error("file ID must not be encoded as an image_url part")
		}
	}
}
