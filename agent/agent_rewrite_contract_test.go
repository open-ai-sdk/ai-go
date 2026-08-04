package agent_test

import (
	"context"
	"iter"
	"testing"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
)

func requireRunSignature(run func(context.Context) (*agent.Result, error)) {}

func requireStreamSignature(
	stream func(context.Context) (iter.Seq2[aikit.StepEvent, error], error),
) {
}

func TestAgentRunnerContractCompiles(t *testing.T) {
	t.Parallel()

	configured, err := agent.New(&runnerScriptModel{}).
		ID("contract-agent").
		Instructions("Follow the ordered conversation history.").
		MaxTurns(4).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	runner := configured.Runner().
		Messages(aikit.SystemMessage("System context")).
		Message(aikit.UserMessage("Describe this image")).
		Prompt("Continue")
	requireRunSignature(runner.Run)
	requireStreamSignature(runner.Stream)
}
