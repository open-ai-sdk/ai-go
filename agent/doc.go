// Package agent builds immutable language-model agents and executes isolated
// runs. New creates a value-style Builder; Build snapshots reusable defaults;
// Agent.Runner creates a per-invocation Runner. Runner.Run aggregates a Result,
// while Runner.Stream exposes the same driver as a single-use event iterator.
//
// The package owns no conversation store. Requests, tool results, and approval
// decisions are carried by values supplied to a Runner, so a caller can
// suspend a request and resume later from message history without retaining
// server-side agent state. Hooks are ordered, run-local lifecycle extensions:
// small capability interfaces cover request/response, model-turn, tool,
// invalid-call, streaming, and finish events. A model-turn hook may retry a
// tool-free response; each retry consumes the normal Runner turn budget and
// buffers that turn until acceptance. HookContext has a scratchpad safe for
// parallel tool callbacks, but neither hooks nor scratchpad replace tool
// authorization.
package agent
