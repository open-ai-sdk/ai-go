package aisdkhttp

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aisdk"
)

func TestWriteStreamEmitsDoneForFinishChunk(t *testing.T) {
	chunks := make(chan aisdk.Chunk, 1)
	chunks <- aisdk.Chunk{Type: aisdk.ChunkFinish, Fields: map[string]any{"finishReason": "stop"}}
	close(chunks)
	recorder := httptest.NewRecorder()
	if err := WriteStream(recorder, chunks); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(recorder.Body.String(), "data: [DONE]\n\n") {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}
