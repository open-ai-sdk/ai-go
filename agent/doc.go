// Package agent builds immutable language-model agents and executes isolated
// runs. New creates a value-style Builder; Build snapshots reusable defaults;
// Agent.Runner creates a per-invocation Runner. Runner.Run aggregates a Result,
// while Runner.Stream exposes the same driver as a single-use event iterator.
//
// The package owns no conversation store. Requests, tool results, and approval
// decisions are carried by values supplied to a Runner, so a caller can
// suspend a request and resume later from message history without retaining
// server-side agent state. Hooks are ordered, run-local lifecycle extensions;
// their HookContext scratchpad is safe for parallel tool callbacks but is not
// a substitute for tool authorization.
package agent
