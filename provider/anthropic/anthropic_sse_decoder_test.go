package anthropic

import (
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestUnknownStreamingContentBlockIsReportedAtFinish(t *testing.T) {
	blocks := make(map[int]*blockState)
	warnings := []aikit.Warning{{Type: "unsupported-setting", Setting: "request"}}
	var events []aikit.StreamEvent
	send := func(event aikit.StreamEvent) bool {
		events = append(events, event)
		return true
	}

	if !handleContentBlockStart(
		`{"index":0,"content_block":{"type":"server_tool_use"}}`,
		blocks,
		send,
		&warnings,
	) {
		t.Fatal("handleContentBlockStart() stopped unexpectedly")
	}
	if !handleMessageDelta(
		`{"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
		"msg_1",
		send,
		warnings,
	) {
		t.Fatal("handleMessageDelta() stopped unexpectedly")
	}

	finish := events[len(events)-1]
	if finish.Type != aikit.StreamEventFinish {
		t.Fatalf("last event = %q, want finish", finish.Type)
	}
	if len(finish.Warnings) != 2 || finish.Warnings[1].Setting != "server_tool_use" {
		t.Fatalf("warnings = %#v, want request and unknown-block warnings", finish.Warnings)
	}
}
