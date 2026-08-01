package aisdk

import (
	"bytes"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestBrowserBoundErrorPathsAreRedacted(t *testing.T) {
	const secret = "org-secret request-body-leak"
	tests := []struct {
		name  string
		write func(*Writer) error
	}{
		{"error chunk", func(writer *Writer) error { return writer.WriteError(secret) }},
		{"tool output error", func(writer *Writer) error {
			return writer.WriteToolOutputError("call-1", secret, nil)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := test.write(NewWriter(&output)); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), secret) || !strings.Contains(output.String(), "stream error") {
				t.Fatalf("unredacted browser payload: %s", output.String())
			}
		})
	}
}

func TestProducerRedactionSurvivesSSESerialization(t *testing.T) {
	events := make(chan aikit.StepEvent, 1)
	events <- aikit.StepEvent{Type: aikit.StepEventError, Error: aikit.NewAPIError("provider-secret", 429, nil)}
	close(events)
	var output bytes.Buffer
	if err := WriteSSEStream(&output, NewChunkProducer("message-1").Produce(events).Chunks); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "provider error (status 429)") ||
		strings.Contains(got, "provider-secret") {
		t.Fatalf("provider error was leaked or double-redacted: %s", got)
	}
}

func TestExecuteErrorTerminatesStream(t *testing.T) {
	var output bytes.Buffer
	Execute(&output, StreamOptions{MessageID: "message-1"}, func(*Writer) error {
		return aikit.NewAPIError("provider-secret", 503, nil)
	})
	got := output.String()
	if !strings.Contains(got, `"finishReason":"error"`) || !strings.Contains(got, "data: [DONE]") ||
		!strings.Contains(got, "provider error (status 503)") {
		t.Fatalf("managed error stream was not terminated: %s", got)
	}
	if strings.Contains(got, "provider-secret") {
		t.Fatalf("provider detail leaked: %s", got)
	}
}

func TestRecoverToChunkEmitsErrorFinish(t *testing.T) {
	chunks := make(chan Chunk, 2)
	recoverToTerminalChunks(chunks)(aikit.NewAPIError("provider-secret", 500, nil))
	close(chunks)
	var output bytes.Buffer
	if err := WriteSSEStream(&output, chunks); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "provider error (status 500)") || !strings.Contains(got, "data: [DONE]") {
		t.Fatalf("recovered stream was not safely terminated: %s", got)
	}
}
