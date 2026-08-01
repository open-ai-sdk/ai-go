package anthropic

import (
	"encoding/base64"
	"testing"

	"github.com/open-ai-sdk/ai-go/ai"
)

func userMessage(parts ...ai.ContentPart) []ai.Message {
	return []ai.Message{{Role: ai.RoleUser, Content: parts}}
}

// Images must reach Anthropic as image blocks regardless of whether the caller
// supplied a full media type or the bare "image" top-level segment.
func TestEncodeMessages_ImageDataBecomesImageBlock(t *testing.T) {
	data := []byte("fake-png-bytes")
	for _, mediaType := range []string{"image/png", "image"} {
		msgs, warnings := encodeMessages(userMessage(ai.ImageDataPart(data, mediaType)))
		if len(warnings) != 0 {
			t.Fatalf("mediaType %q: unexpected warnings: %+v", mediaType, warnings)
		}
		if len(msgs) != 1 || len(msgs[0].Content) != 1 {
			t.Fatalf("mediaType %q: expected 1 message with 1 block, got %+v", mediaType, msgs)
		}
		block := msgs[0].Content[0]
		if block.Type != "image" {
			t.Errorf("mediaType %q: expected block type image, got %q", mediaType, block.Type)
		}
		if block.Source == nil || block.Source.MediaType != mediaType {
			t.Errorf("mediaType %q: unexpected source %+v", mediaType, block.Source)
		}
		want := base64.StdEncoding.EncodeToString(data)
		if block.Source.Data != want {
			t.Errorf("mediaType %q: expected base64 data %q, got %q", mediaType, want, block.Source.Data)
		}
	}
}

// A PDF is a document block, not an image block — Anthropic rejects PDFs sent
// as images, so encoding one as an image would be a silent 400.
func TestEncodeMessages_PDFBecomesDocumentBlock(t *testing.T) {
	msgs, warnings := encodeMessages(userMessage(
		ai.FileDataPart([]byte("%PDF-1.4"), "application/pdf", "report.pdf"),
	))
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	if len(msgs) != 1 || len(msgs[0].Content) != 1 {
		t.Fatalf("expected 1 message with 1 block, got %+v", msgs)
	}
	if got := msgs[0].Content[0].Type; got != "document" {
		t.Errorf("expected block type document, got %q", got)
	}
}

// Unsupported media types are dropped with a warning rather than silently
// vanishing or being mislabelled as an image.
func TestEncodeMessages_UnsupportedMediaTypeWarns(t *testing.T) {
	msgs, warnings := encodeMessages(userMessage(
		ai.FileDataPart([]byte("zip-bytes"), "application/zip", "archive.zip"),
	))
	if len(msgs) != 0 {
		t.Errorf("expected the message to be dropped, got %+v", msgs)
	}
	if len(warnings) != 1 || warnings[0].Setting != "mediaType" {
		t.Fatalf("expected one mediaType warning, got %+v", warnings)
	}
}

// The Messages API cannot reference a URL or a provider file ID, so those parts
// must warn instead of disappearing from the request.
func TestEncodeMessages_UnreferenceableFilePartsWarn(t *testing.T) {
	tests := []struct {
		name        string
		part        ai.ContentPart
		wantSetting string
	}{
		{"url", ai.ImageURLPart("https://example.com/img.png"), "fileURL"},
		{"fileID", ai.ImageFileIDPart("file-abc123"), "fileID"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs, warnings := encodeMessages(userMessage(ai.TextPart("look"), tc.part))
			if len(warnings) != 1 || warnings[0].Setting != tc.wantSetting {
				t.Fatalf("expected one %s warning, got %+v", tc.wantSetting, warnings)
			}
			// The text part must survive so the turn is not lost entirely.
			if len(msgs) != 1 || len(msgs[0].Content) != 1 || msgs[0].Content[0].Type != "text" {
				t.Errorf("expected the text block to survive, got %+v", msgs)
			}
		})
	}
}
