package agent_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
)

func TestModelTurnHookRetriesWithoutPublishingRejectedStreamDeltas(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{
		runnerTextEvents("reject-id", "reject"),
		runnerTextEvents("accept-id", "accept"),
	}}
	var attempts atomic.Int32
	hook := agent.HookFuncs{Name: "quality", ModelTurnFunc: func(
		_ context.Context,
		_ agent.HookContext,
		event agent.ModelTurnEvent,
	) (agent.ModelTurnAction, error) {
		if attempts.Add(1) == 1 {
			if event.HasToolCalls {
				t.Fatal("text fixture unexpectedly has tool calls")
			}
			return agent.ModelTurnAction{Kind: agent.ModelTurnRetry, Retry: agent.Repeat()}, nil
		}
		return agent.ModelTurnAction{Kind: agent.ModelTurnContinue}, nil
	}}
	built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
		return builder.Hook(hook).MaxTurns(2)
	})
	events, err := built.Runner().Prompt("go").Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var starts int
	for event, streamErr := range events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if event.Type == aikit.StepEventTextDelta {
			text += event.TextDelta
		}
		if event.Type == aikit.StepEventStepStart {
			starts++
		}
	}
	if text != "accept" {
		t.Fatalf("streamed text = %q, want only accepted turn", text)
	}
	if got := len(model.requestSnapshots()); got != 2 {
		t.Fatalf("model calls = %d, want 2", got)
	}
	if starts != 1 {
		t.Fatalf("step starts = %d, want only accepted turn", starts)
	}
}

func TestModelTurnStopKeepsHookProvenance(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{runnerTextEvents("draft-id", "draft")}}
	hook := agent.HookFuncs{Name: "quality", ModelTurnFunc: func(
		_ context.Context,
		_ agent.HookContext,
		_ agent.ModelTurnEvent,
	) (agent.ModelTurnAction, error) {
		return agent.ModelTurnAction{Kind: agent.ModelTurnStop, Reason: "draft rejected"}, nil
	}}
	built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder { return builder.Hook(hook) })
	_, err := built.Runner().Prompt("go").Run(context.Background())
	var hookErr *agent.HookError
	if !errors.As(err, &hookErr) || hookErr.Hook != "quality" || hookErr.Phase != "model_turn" {
		t.Fatalf("error = %#v", err)
	}
}

func TestModelTurnRetryWithToolCallsIsTypedHookError(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{
		{
			{
				Type: aikit.StreamEventToolCallDelta, ToolCallIndex: 0,
				ToolCallID: "call-1", ToolCallName: "lookup", ToolCallArgsDelta: `{}`,
			},
			{
				Type: aikit.StreamEventFinish, MessageID: "tool-id", FinishReason: aikit.FinishReasonToolCalls,
			},
		},
	}}
	hook := agent.HookFuncs{Name: "policy", ModelTurnFunc: func(
		_ context.Context,
		_ agent.HookContext,
		_ agent.ModelTurnEvent,
	) (agent.ModelTurnAction, error) {
		return agent.ModelTurnAction{Kind: agent.ModelTurnRetry, Retry: agent.Repeat()}, nil
	}}
	built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder { return builder.Hook(hook) })
	_, err := built.Runner().Prompt("go").Run(context.Background())
	var hookErr *agent.HookError
	if !errors.As(err, &hookErr) || hookErr.Hook != "policy" || hookErr.Phase != "model_turn" {
		t.Fatalf("error = %#v", err)
	}
}

func TestModelTurnHookFeedbackAddsCorrectiveHistory(t *testing.T) {
	model := &runnerScriptModel{scripts: [][]aikit.StreamEvent{
		runnerTextEvents("draft-id", "draft"),
		runnerTextEvents("final-id", "final"),
	}}
	var first atomic.Bool
	hook := agent.HookFuncs{Name: "quality", ModelTurnFunc: func(
		_ context.Context,
		_ agent.HookContext,
		_ agent.ModelTurnEvent,
	) (agent.ModelTurnAction, error) {
		if first.CompareAndSwap(false, true) {
			return agent.ModelTurnAction{
				Kind: agent.ModelTurnRetry, Retry: agent.RetryWithFeedback("be more precise"),
			}, nil
		}
		return agent.ModelTurnAction{Kind: agent.ModelTurnContinue}, nil
	}}
	built := mustRunnerAgent(t, model, func(builder agent.Builder) agent.Builder {
		return builder.Hook(hook).MaxTurns(2)
	})
	result, err := built.Runner().Prompt("go").Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "final" {
		t.Fatalf("result text = %q", result.Text)
	}
	requests := model.requestSnapshots()
	if len(requests) != 2 || len(requests[1].Messages) < 3 {
		t.Fatalf("retry request = %#v", requests)
	}
	if got := requests[1].Messages[len(requests[1].Messages)-1].Content[0].Text; got != "be more precise" {
		t.Fatalf("feedback = %q", got)
	}
}
