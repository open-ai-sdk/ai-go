package ainode

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

// The Protocol encoder used by aisdkhttp.Handler once dropped file, structured
// output, and usage events that the channel-based paths already handled. These
// tests pin the full event set to the wire.
func TestProtocolEncodesEveryPublishableEvent(t *testing.T) {
	stream := pipeEvents(t, eventSeq(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hi"},
		aikit.StepEvent{
			Type: aikit.StepEventFileDelta, FileData: []byte("PNGDATA"), FileMediaType: "image/png",
		},
		aikit.StepEvent{
			Type:   aikit.StepEventSource,
			Source: &aikit.Source{SourceType: "url", ID: "s1", URL: "https://x.dev", Title: "X"},
		},
		aikit.StepEvent{
			Type:  aikit.StepEventUsage,
			Usage: &aikit.Usage{InputTokens: 11, OutputTokens: 22, TotalTokens: 33},
		},
		aikit.StepEvent{Type: aikit.StepEventStructuredOutput, StructuredOutput: []byte(`{"a":1}`)},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))

	want := []string{
		"start", "start-step", "text-start", "text-delta", "file", "source-url",
		"data-structured-output", "text-end", "finish-step", "finish",
	}
	if got := wireChunkTypes(t, stream); !equalChunkTypes(got, want) {
		t.Fatalf("chunk order = %v, want %v\n%s", got, want, stream)
	}
	for _, chunkType := range want {
		if !ValidChunkType(chunkType) {
			t.Errorf("%s is not a valid v7 chunk type", chunkType)
		}
	}
}

func TestProtocolPublishesFileAsDataURL(t *testing.T) {
	stream := pipeEvents(t, eventSeq(
		aikit.StepEvent{
			Type: aikit.StepEventFileDelta, FileData: []byte("PNGDATA"), FileMediaType: "image/png",
		},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))
	file := chunkOfType(t, stream, "file")
	if file["url"] != "data:image/png;base64,UE5HREFUQQ==" || file["mediaType"] != "image/png" {
		t.Fatalf("file chunk = %#v", file)
	}
	// v7's file chunk has no filename field; adding one would break v5 clients.
	if _, present := file["filename"]; present {
		t.Errorf("file chunk carries a filename: %#v", file)
	}
}

// v7 removed every usage field from the chunk schema, so token counts can only
// reach useChat's onFinish through messageMetadata.
func TestProtocolPublishesUsageThroughMessageMetadata(t *testing.T) {
	stream := pipeEvents(t, eventSeq(
		aikit.StepEvent{Type: aikit.StepEventStepStart},
		aikit.StepEvent{
			Type: aikit.StepEventUsage,
			Usage: &aikit.Usage{
				InputTokens: 11, OutputTokens: 22, TotalTokens: 33,
				OutputTokenDetails: aikit.OutputTokenDetails{ReasoningTokens: 7},
			},
		},
		aikit.StepEvent{Type: aikit.StepEventStepEnd, FinishReason: aikit.FinishReasonStop},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))
	finish := chunkOfType(t, stream, "finish")
	metadata, ok := finish["messageMetadata"].(map[string]any)
	if !ok {
		t.Fatalf("finish chunk carries no messageMetadata: %#v", finish)
	}
	usage, ok := metadata["usage"].(map[string]any)
	if !ok {
		t.Fatalf("messageMetadata carries no usage: %#v", metadata)
	}
	if usage["inputTokens"] != float64(11) || usage["outputTokens"] != float64(22) ||
		usage["totalTokens"] != float64(33) {
		t.Fatalf("usage = %#v", usage)
	}
	details, ok := usage["outputTokenDetails"].(map[string]any)
	if !ok || details["reasoningTokens"] != float64(7) {
		t.Fatalf("outputTokenDetails = %#v", usage["outputTokenDetails"])
	}
}

func TestProtocolOmitsUsageMetadataWhenUnreported(t *testing.T) {
	stream := pipeEvents(t, eventSeq(
		aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hi"},
		aikit.StepEvent{Type: aikit.StepEventDone},
	))
	finish := chunkOfType(t, stream, "finish")
	if _, present := finish["messageMetadata"]; present {
		t.Fatalf("finish chunk carries empty metadata: %#v", finish)
	}
}

func pipeEvents(t *testing.T, events iter.Seq2[aikit.StepEvent, error]) string {
	t.Helper()
	var output bytes.Buffer
	err := uistream.Pipe(
		context.Background(), &output, events, Protocol(),
		uistream.Options{MessageID: "msg-1"},
	)
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	return output.String()
}

func eventSeq(events ...aikit.StepEvent) iter.Seq2[aikit.StepEvent, error] {
	return func(yield func(aikit.StepEvent, error) bool) {
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func wireChunkTypes(t *testing.T, stream string) []string {
	t.Helper()
	var types []string
	for _, chunk := range decodeChunks(t, stream) {
		if value, ok := chunk["type"].(string); ok {
			types = append(types, value)
		}
	}
	return types
}

func chunkOfType(t *testing.T, stream, want string) map[string]any {
	t.Helper()
	for _, chunk := range decodeChunks(t, stream) {
		if chunk["type"] == want {
			return chunk
		}
	}
	t.Fatalf("stream has no %s chunk:\n%s", want, stream)
	return nil
}

func decodeChunks(t *testing.T, stream string) []map[string]any {
	t.Helper()
	var chunks []map[string]any
	for _, line := range strings.Split(stream, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		chunk := map[string]any{}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			t.Fatalf("decode chunk %q: %v", payload, err)
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func equalChunkTypes(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
