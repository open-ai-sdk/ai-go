package generate

import "context"

// ToolLoopAgent is the reference Agent implementation. It binds a model,
// tools, and step-loop defaults, then delegates each Generate/Stream call to
// GenerateText/StreamText — it runs no loop of its own.
//
// The loop continues until: a finish reason other than tool-calls is
// returned, a called tool has no executor, a tool call needs approval, or a
// stop condition is met. Unlike GenerateText/StreamText's bare default of
// IsStepCount(1), ToolLoopAgent defaults to IsStepCount(20): an agent is
// expected to run a real multi-step loop out of the box, while a one-off
// GenerateText call is not.
type ToolLoopAgent struct {
	id       string
	settings GenerateTextRequest
}

var (
	_ Agent      = (*ToolLoopAgent)(nil)
	_ Completion = (*ToolLoopAgent)(nil)
)

// NewToolLoopAgent creates a ToolLoopAgent bound to model, configured by opts.
// Messages are never set here — they are per-call, supplied via WithMessages
// (or another Option) to Generate/Stream.
func NewToolLoopAgent(model LanguageModel, opts ...AgentOption) *ToolLoopAgent {
	a := &ToolLoopAgent{settings: GenerateTextRequest{Model: model}}
	for _, o := range opts {
		o(a)
	}
	return a
}

// ID returns the agent's identifier, or "" if WithAgentID was never used.
func (a *ToolLoopAgent) ID() string { return a.id }

// Tools returns the tool set the agent calls by default, or nil.
func (a *ToolLoopAgent) Tools() *ToolSet { return a.settings.Tools }

// Generate runs a full tool loop and returns the aggregated result. opts
// override the agent's defaults for this call only — scalar fields (model,
// tools, stop condition, settings, ...) are replaced outright, while the
// OnStepEnd/OnEnd/OnChunk/OnError callbacks merge with the agent's own instead
// of replacing them (see mergeCallback in agent-options.go).
func (a *ToolLoopAgent) Generate(ctx context.Context, opts ...Option) (*GenerateTextResult, error) {
	return GenerateText(ctx, a.mergeRequest(opts))
}

// Stream runs the tool loop and returns a *StreamResult for live streaming.
// The error return exists for Agent implementations that can fail before a
// stream starts; ToolLoopAgent never populates it — a pre-flight validation
// failure surfaces through the returned StreamResult itself (its Consume/
// Stream views deliver the error), matching StreamText's own contract.
func (a *ToolLoopAgent) Stream(ctx context.Context, opts ...Option) (*StreamResult, error) {
	return StreamText(ctx, a.mergeRequest(opts)), nil
}
