package agui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/uistream"
)

func TestProtocolProducesRunLifecycle(t *testing.T) {
	var b bytes.Buffer
	p := Protocol(WithRunID(func() string { return "run_1" }))
	events := func(yield func(aikit.StepEvent, error) bool) {
		yield(aikit.StepEvent{Type: aikit.StepEventTextDelta, TextDelta: "hello"}, nil)
		yield(aikit.StepEvent{Type: aikit.StepEventDone}, nil)
	}
	if err := uistream.Pipe(context.Background(), &b, events, p, uistream.Options{}); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	for _, want := range []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END", "RUN_FINISHED"} {
		if !strings.Contains(got, want) {
			t.Errorf("stream missing %s: %s", want, got)
		}
	}
}
