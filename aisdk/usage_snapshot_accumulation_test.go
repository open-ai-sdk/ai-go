package aisdk

import (
	"bytes"
	"strings"
	"testing"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func cumulativeUsageEvents() []aikit.StepEvent {
	return []aikit.StepEvent{
		{Type: aikit.StepEventStepStart, StepNumber: 0},
		{Type: aikit.StepEventUsage, Usage: &aikit.Usage{
			InputTokens: 10, OutputTokens: 5, TotalTokens: 15,
		}},
		{Type: aikit.StepEventUsage, Usage: &aikit.Usage{
			InputTokens: 10, OutputTokens: 20, TotalTokens: 30,
		}},
		{Type: aikit.StepEventStepEnd, StepNumber: 0, FinishReason: aikit.FinishReasonStop},
		{Type: aikit.StepEventDone},
	}
}

func TestToUIMessageStream_CumulativeUsageUsesLatestStepSnapshot(t *testing.T) {
	stream := newEventStream(cumulativeUsageEvents()...)
	var captured MessageMetadataInfo
	chunks := drainChunks(ToUIMessageStream(stream, "message-1", ToUIStreamOptions{
		MessageMetadata: func(info MessageMetadataInfo) map[string]any {
			captured = info
			return nil
		},
	}))

	if _, ok := findChunk(chunks, ChunkFinish); !ok {
		t.Fatal("expected finish chunk")
	}
	if captured.Usage == nil {
		t.Fatal("expected usage metadata")
	}
	if got, want := captured.Usage.InputTokens, 10; got != want {
		t.Fatalf("InputTokens = %d, want %d", got, want)
	}
	if got, want := captured.Usage.OutputTokens, 20; got != want {
		t.Fatalf("OutputTokens = %d, want %d", got, want)
	}
	if got, want := captured.Usage.TotalTokens, 30; got != want {
		t.Fatalf("TotalTokens = %d, want %d", got, want)
	}
}

func TestAdapter_CumulativeUsageUsesLatestStepSnapshot(t *testing.T) {
	var output bytes.Buffer
	NewAdapter("message-1").Stream(newEventStream(cumulativeUsageEvents()...), &output)
	body := output.String()

	if !strings.Contains(body, `"inputTokens":10`) {
		t.Fatalf("finish metadata missing inputTokens=10: %s", body)
	}
	if !strings.Contains(body, `"outputTokens":20`) {
		t.Fatalf("finish metadata missing outputTokens=20: %s", body)
	}
	if !strings.Contains(body, `"totalTokens":30`) {
		t.Fatalf("finish metadata missing totalTokens=30: %s", body)
	}
}

func TestUsageAccumulator_MultiStepDetails(t *testing.T) {
	var accumulator usageAccumulator
	accumulator.startStep()
	accumulator.apply(&aikit.Usage{
		InputTokens: 10,
		InputTokenDetails: aikit.InputTokenDetails{
			NoCacheTokens:   8,
			CacheReadTokens: 2,
		},
		OutputTokens: 5,
		OutputTokenDetails: aikit.OutputTokenDetails{
			TextTokens:      3,
			ReasoningTokens: 2,
		},
		TotalTokens: 15,
	})
	accumulator.apply(&aikit.Usage{
		InputTokens: 10,
		InputTokenDetails: aikit.InputTokenDetails{
			NoCacheTokens:    7,
			CacheReadTokens:  2,
			CacheWriteTokens: 1,
		},
		OutputTokens: 20,
		OutputTokenDetails: aikit.OutputTokenDetails{
			TextTokens:      12,
			ReasoningTokens: 8,
		},
		TotalTokens: 30,
	})
	accumulator.startStep()
	accumulator.apply(&aikit.Usage{
		InputTokens: 4,
		InputTokenDetails: aikit.InputTokenDetails{
			NoCacheTokens:   3,
			CacheReadTokens: 1,
		},
		OutputTokens: 6,
		OutputTokenDetails: aikit.OutputTokenDetails{
			TextTokens:      4,
			ReasoningTokens: 2,
		},
		TotalTokens: 10,
	})

	usage := accumulator.snapshot()
	if usage.InputTokens != 14 || usage.OutputTokens != 26 || usage.TotalTokens != 40 {
		t.Fatalf("total usage = %+v, want input=14 output=26 total=40", usage)
	}
	if usage.InputTokenDetails.NoCacheTokens != 10 ||
		usage.InputTokenDetails.CacheReadTokens != 3 ||
		usage.InputTokenDetails.CacheWriteTokens != 1 {
		t.Fatalf("input details = %+v, want no-cache=10 read=3 write=1", usage.InputTokenDetails)
	}
	if usage.OutputTokenDetails.TextTokens != 16 ||
		usage.OutputTokenDetails.ReasoningTokens != 10 {
		t.Fatalf("output details = %+v, want text=16 reasoning=10", usage.OutputTokenDetails)
	}
}
