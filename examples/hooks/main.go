// Package main demonstrates a run-local hook without requiring credentials.
package main

import (
	"context"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
)

func main() {
	_ = agent.HookFuncs{
		Name: "audit",
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
	}
}
