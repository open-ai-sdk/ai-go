package agent_test

import (
	"context"
	"fmt"
	"log"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
)

// The functions below are the snippets docs/core/streaming.md and
// docs/core/agent-runner.md publish. They live here so the build fails when the
// docs go stale, rather than a reader discovering it.

// docStreamAgentRun is the "Agent stream" snippet.
func docStreamAgentRun(ctx context.Context, assistant *agent.Agent) error {
	events, err := assistant.Runner().
		Prompt("Explain Go channels").
		MaxTurns(4).
		Stream(ctx)
	if err != nil {
		return err // synchronous Runner validation
	}

	for event, err := range events {
		if err != nil {
			return err
		}
		switch event.Type {
		case aikit.StepEventTextDelta:
			fmt.Print(event.TextDelta)
		case aikit.StepEventToolResult:
			log.Printf("tool %s completed", event.ToolCallName)
		}
	}
	return nil
}

// docStreamAgentRunWithResult is the "Streaming and the aggregate together"
// snippet.
func docStreamAgentRunWithResult(ctx context.Context, assistant *agent.Agent) error {
	stream, err := assistant.Runner().
		Prompt("Explain Go channels").
		MaxTurns(4).
		StreamRun(ctx)
	if err != nil {
		return err
	}

	for event, err := range stream.Events() {
		if err != nil {
			return err
		}
		if event.Type == aikit.StepEventTextDelta {
			fmt.Print(event.TextDelta)
		}
	}

	result, err := stream.Result()
	if err != nil {
		return err
	}
	fmt.Println(len(result.Steps), result.Usage.TotalTokens)
	return nil
}

// docStreamAgentPrompt is the "Agent prompt and chat" snippet.
func docStreamAgentPrompt(ctx context.Context, assistant *agent.Agent, history []aikit.Message) error {
	stream, err := assistant.StreamChat(ctx, "And in one sentence?", history...)
	if err != nil {
		return err
	}
	for event, err := range stream.Events() {
		if err != nil {
			return err
		}
		if event.Type == aikit.StepEventTextDelta {
			fmt.Print(event.TextDelta)
		}
	}
	_, err = stream.Result()
	return err
}

func TestDocumentedAgentStreamSnippetsRun(t *testing.T) {
	ctx := context.Background()
	history := []aikit.Message{aikit.UserMessage("Explain Go channels")}

	assistant := mustRunnerAgent(t, &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}})
	if err := docStreamAgentRun(ctx, assistant); err != nil {
		t.Errorf("docStreamAgentRun() error = %v", err)
	}

	assistant = mustRunnerAgent(t, &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}})
	if err := docStreamAgentRunWithResult(ctx, assistant); err != nil {
		t.Errorf("docStreamAgentRunWithResult() error = %v", err)
	}

	assistant = mustRunnerAgent(t, &runnerScriptModel{scripts: [][]aikit.StreamEvent{richRunScript()}})
	if err := docStreamAgentPrompt(ctx, assistant, history); err != nil {
		t.Errorf("docStreamAgentPrompt() error = %v", err)
	}
}
