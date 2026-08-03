package agent_test

import (
	"context"
	"iter"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/llm"
	"github.com/open-ai-sdk/ai-go/tool"
)

// compileAgentRunnerContract is a build-only probe for the clean-break Agent
// API. It intentionally compiles as part of the normal suite so the canonical
// surface cannot drift unnoticed.
func compileAgentRunnerContract(model llm.Model, tools *tool.Set, message aikit.Message) error {
	assistant, err := agent.New(model).
		ID("contract-agent").
		Instructions("Follow the ordered conversation history.").
		Tools(tools).
		MaxTurns(4).
		Build()
	if err != nil {
		return err
	}

	runner := assistant.Runner().
		Messages(
			aikit.SystemMessage("System context"),
			aikit.Message{
				Role: aikit.RoleUser,
				Content: []aikit.ContentPart{
					aikit.TextPart("Describe this image"),
					aikit.ImageURLPart("https://example.com/image.png"),
				},
			},
		).
		Message(message).
		Prompt("Continue")

	var run func(context.Context) (*agent.Result, error) = runner.Run
	var stream func(context.Context) (iter.Seq2[aikit.StepEvent, error], error) = runner.Stream
	_, _ = run, stream
	return nil
}
