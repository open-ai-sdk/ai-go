package ainode

import (
	"bytes"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestBrowserBoundErrorPathsAreRedacted(t *testing.T) {
	const secret = "org-secret request-body-leak"
	var output bytes.Buffer
	if err := NewWriter(&output).WriteError(secret); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), secret) || !strings.Contains(output.String(), "stream error") {
		t.Fatalf("unredacted browser payload: %s", output.String())
	}
}

// Tool failure text is app-authored, in the same trust domain as the tool's
// success output, which already reaches the wire verbatim. Redacting it would
// make the output-error state display-useless.
func TestToolOutputErrorTextReachesTheWireUnredacted(t *testing.T) {
	const detail = "order 42 not found in region eu-west-1"
	var output bytes.Buffer
	if err := NewWriter(&output).WriteToolOutputError("call-1", detail, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), detail) {
		t.Fatalf("tool errorText was redacted: %s", output.String())
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
