package agent

import (
	"context"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// StreamPrompt streams one prompt as a complete agent run — tools, approvals,
// hooks, and stop conditions included — and carries the *Result the run
// aggregates.
//
// Note the deliberate asymmetry with the model layer: an agent streams the
// multi-turn vocabulary (StepEvent), a model streams the assistant-content
// vocabulary (StreamEvent). Agent does not gain a StreamEvent stream.
func (a *Agent) StreamPrompt(ctx context.Context, prompt string) (*StepStream, error) {
	return a.Runner().Prompt(prompt).StreamRun(ctx)
}

// StreamChat streams one prompt after history as a complete agent run. History
// is copied into the run; it is never mutated.
func (a *Agent) StreamChat(
	ctx context.Context,
	prompt string,
	history ...aikit.Message,
) (*StepStream, error) {
	return a.Runner().Messages(history...).Prompt(prompt).StreamRun(ctx)
}

// StreamCompletion returns a Runner rather than a stream, so the caller can
// shape the invocation — max turns, active tools, hooks — before starting it.
//
// Unlike the other two it reports nothing here: Runner carries its validation
// error until StreamRun or Run is called, which is the existing Runner
// contract. The error return exists to satisfy the interface.
func (a *Agent) StreamCompletion(
	_ context.Context,
	prompt string,
	history ...aikit.Message,
) (Runner, error) {
	return a.Runner().Messages(history...).Prompt(prompt), nil
}

// The build enforces the contract: this is the StepEvent instantiation that
// gives the aikit streaming interfaces a second implementer.
var (
	_ aikit.StreamingPrompt[aikit.StepEvent, *StepStream] = (*Agent)(nil)
	_ aikit.StreamingChat[aikit.StepEvent, *StepStream]   = (*Agent)(nil)
	_ aikit.StreamingCompletion[Runner]                   = (*Agent)(nil)
	_ aikit.Stream[aikit.StepEvent]                       = (*StepStream)(nil)
)
