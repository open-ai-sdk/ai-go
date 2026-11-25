package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/tool"
)

func TestRunnerHostCanReplayReceiptOrRequireReconciliation(t *testing.T) {
	const callID = "call-1"
	const toolName = "record_change"
	reconcile := errors.New("operation requires reconciliation")
	for _, tc := range []struct {
		name       string
		receipt    string
		wantLaunch int32
		uncertain  bool
	}{
		{name: "fresh", wantLaunch: 1},
		{name: "recorded", receipt: `{"receipt":"saved"}`},
		{name: "uncertain", uncertain: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var launches atomic.Int32
			handler, err := tool.NewDynamic(toolName, "Record a change", map[string]any{"type": "object"},
				func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
					if id := tool.ToolCallIDFromContext(ctx); id != callID || string(args) != `{}` {
						return nil, &tool.AbortRunError{Cause: errors.New("unexpected call identity")}
					}
					if tc.uncertain {
						return nil, &tool.AbortRunError{Cause: reconcile}
					}
					if tc.receipt != "" {
						return json.RawMessage(tc.receipt), nil
					}
					launches.Add(1)
					return json.RawMessage(`{"receipt":"fresh"}`), nil
				})
			if err != nil {
				t.Fatal(err)
			}
			tools, err := tool.NewSet(handler)
			if err != nil {
				t.Fatal(err)
			}
			model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{
				{
					{
						Type:              aikit.StreamEventToolCallDelta,
						ToolCallID:        callID,
						ToolCallName:      toolName,
						ToolCallArgsDelta: `{}`,
					},
					{Type: aikit.StreamEventFinish, FinishReason: aikit.FinishReasonToolCalls},
				},
				runnerTextEvents("answer", "Complete."),
			}}
			built := mustRunnerAgent(
				t,
				model,
				func(b agent.Builder) agent.Builder { return b.Tools(tools).MaxTurns(2) },
			)
			result, err := built.Runner().Prompt("Record the change").Run(t.Context())
			requests := model.requestSnapshots()
			if got := launches.Load(); got != tc.wantLaunch {
				t.Fatalf("launches=%d, want %d", got, tc.wantLaunch)
			}
			if tc.uncertain {
				if !errors.Is(err, reconcile) || len(requests) != 1 {
					t.Fatalf("uncertain operation was not terminal: calls=%d, error=%v", len(requests), err)
				}
				return
			}
			if err != nil || result == nil || len(requests) != 2 {
				t.Fatalf("completion: calls=%d, error=%v", len(requests), err)
			}
			want := tc.receipt
			if want == "" {
				want = `{"receipt":"fresh"}`
			}
			var got string
			for _, message := range requests[1].Messages {
				for _, part := range message.Content {
					if part.Type == aikit.ContentPartTypeToolResult && part.ToolResultID == callID {
						got = part.ToolResultOutput
					}
				}
			}
			if got != want {
				t.Fatalf("model got %q, want %q", got, want)
			}
		})
	}
}
