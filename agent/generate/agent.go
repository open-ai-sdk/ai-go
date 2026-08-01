package generate

import "context"

// Agent receives a prompt (supplied via Option, e.g. WithMessages) and
// generates or streams an output made of steps, tool calls, and other
// step-level data.
//
// An Agent is a facade: it binds a model, tools, and defaults, then delegates
// each call to the stateless GenerateText/StreamText tool loop. It never runs
// a loop of its own — agent.Stream is the only implementation,
// shared by every entry point. An Agent holds no per-call state (steps,
// messages, and tool calls live on the stack of each Generate/Stream call),
// so an implementation is safe for concurrent use provided its tools are.
//
// ToolLoopAgent is the reference implementation. Consumers may implement Agent
// themselves; the interface is deliberately small to make that practical.
//
// This interface fixes the shape to *ToolSet and the existing
// GenerateTextResult/StreamResult types; tool typing stays at tool.New[In, Out].
type Agent interface {
	// ID identifies the agent. Empty when unset.
	ID() string
	// Tools returns the tool set the agent calls by default. Nil if none.
	Tools() *ToolSet
	// Generate runs a full tool loop and returns the aggregated result.
	Generate(ctx context.Context, opts ...Option) (*GenerateTextResult, error)
	// Stream runs the tool loop and returns a *StreamResult for live streaming.
	Stream(ctx context.Context, opts ...Option) (*StreamResult, error)
}
