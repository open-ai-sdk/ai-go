// Package agent runs multi-step language-model tool loops.
//
// Stream accepts the shared llm model/request vocabulary, invokes tools from a
// tool.Set, and emits aikit.StepEvent values until the model finishes, a stop
// condition fires, the step budget is exhausted, or the context is cancelled.
// Run is the same low-level event-stream operation under the blocking-path
// name used by callers that aggregate those events into a result.
//
// The package owns no conversation store. Requests, tool results, and approval
// decisions are carried by values supplied for one invocation, so a caller can
// suspend a request and resume later from message history without retaining
// server-side agent state.
package agent
