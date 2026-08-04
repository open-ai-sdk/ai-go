// Package main demonstrates a run-local hook without requiring credentials.
package main

import (
	"context"
	"strings"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func main() {
	hook := agent.HookFuncs{
		Name: "policy-and-audit",
		BeforeCompletionFunc: func(_ context.Context, hc agent.HookContext, _ llm.Request) (agent.CompletionAction, error) {
			hc.Store("audit-enabled", true)
			return agent.CompletionAction{Kind: agent.CompletionContinue}, nil
		},
		BeforeToolFunc: func(_ context.Context, hc agent.HookContext, call aikit.ToolCallInfo) (agent.ToolCallAction, error) {
			if enabled, _ := agent.ScratchpadGet[bool](hc, "audit-enabled"); enabled && call.Name == "destructive" {
				return agent.ToolCallAction{Kind: agent.ToolCallSkip, Reason: "application policy"}, nil
			}
			return agent.ToolCallAction{Kind: agent.ToolCallRun}, nil
		},
		ToolResultFunc: func(_ context.Context, _ agent.HookContext, event agent.ToolResultEvent) (agent.ToolResultAction, error) {
			// Raw is the execution fact; Presentation is the only rewriteable view.
			if event.Raw.Name != "customer_lookup" {
				return agent.ToolResultAction{Kind: agent.ToolResultKeep}, nil
			}
			shown := event.Presentation
			shown.Output = "Customer record retrieved."
			shown.Content = []aikit.ToolResultContent{aikit.TextToolResultContent(shown.Output)}
			return agent.ToolResultAction{Kind: agent.ToolResultRewrite, Result: shown}, nil
		},
		ModelTurnFunc: func(_ context.Context, hc agent.HookContext, event agent.ModelTurnEvent) (agent.ModelTurnAction, error) {
			// Retry only tool-free turns. The scratchpad limits retries per run.
			if event.HasToolCalls || !strings.Contains(event.Text, "RETRY") {
				return agent.ModelTurnAction{Kind: agent.ModelTurnContinue}, nil
			}
			attempt, _ := agent.ScratchpadGet[int](hc, "retry-attempt")
			attempt++
			hc.Store("retry-attempt", attempt)
			if attempt > 2 {
				return agent.ModelTurnAction{Kind: agent.ModelTurnStop, Reason: "retry limit reached"}, nil
			}
			return agent.ModelTurnAction{
				Kind: agent.ModelTurnRetry, Retry: agent.RetryWithFeedback("Return a complete answer."),
			}, nil
		},
	}
	_ = hook
}
