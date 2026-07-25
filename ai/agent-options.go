package ai

import (
	"sync"

	"github.com/open-ai-sdk/ai-go/internal/safego"
)

// AgentOption configures a ToolLoopAgent at construction time. It sets an
// agent-level default; a per-call Option passed to Generate/Stream overrides
// the corresponding GenerateTextRequest field for that call only (see
// mergeRequest).
type AgentOption func(*ToolLoopAgent)

// WithAgentID sets the agent's identifier, returned by Agent.ID().
func WithAgentID(id string) AgentOption {
	return func(a *ToolLoopAgent) { a.id = id }
}

// WithAgentTools attaches the ToolSet the agent calls by default.
func WithAgentTools(ts *ToolSet) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.Tools = ts }
}

// WithAgentInstructions sets the agent's default system prompt.
func WithAgentInstructions(s string) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.Instructions = s }
}

// WithAgentToolChoice sets the agent's default tool-selection policy.
func WithAgentToolChoice(tc ToolChoice) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.ToolChoice = &tc }
}

// WithAgentStopWhen sets the agent's default stop condition. If never set,
// the agent falls back to IsStepCount(20) at call time (see mergeRequest).
func WithAgentStopWhen(sc StopCondition) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.StopWhen = sc }
}

// WithAgentPrepareStep sets the callback invoked before each tool-loop step.
func WithAgentPrepareStep(fn PrepareStepFunc) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.PrepareStep = fn }
}

// WithAgentOutput constrains the agent's default output to the given schema.
func WithAgentOutput(o *OutputSchema) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.Output = o }
}

// WithAgentProviderOptions attaches default provider-specific options.
func WithAgentProviderOptions(opts map[string]any) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.ProviderOptions = opts }
}

// WithAgentRepairToolCall sets the agent's default tool-call repair function.
func WithAgentRepairToolCall(fn RepairToolCallFunc) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.RepairToolCall = fn }
}

// WithAgentToolApproval sets the agent's default tool approval policies.
func WithAgentToolApproval(approval map[string]ToolApprovalFunc) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.ToolApproval = approval }
}

// WithAgentToolApprovalResponder sets the agent's default approval responder.
func WithAgentToolApprovalResponder(fn ToolApprovalResponder) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.ToolApprovalResponder = fn }
}

// WithAgentParallelToolExecution enables parallel tool-call execution by default.
func WithAgentParallelToolExecution(enabled bool) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.ParallelToolExecution = enabled }
}

// WithAgentOnStepEnd sets the agent-level OnStepEnd callback. A call-level
// OnStepEnd (set via WithOnStepEnd on Generate/Stream) runs concurrently with
// this one; neither affects the other or the run (see mergeCallback).
func WithAgentOnStepEnd(fn func(StepEndEvent)) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.OnStepEnd = fn }
}

// WithAgentOnEnd sets the agent-level OnEnd callback (merge semantics as WithAgentOnStepEnd).
func WithAgentOnEnd(fn func(EndEvent)) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.OnEnd = fn }
}

// WithAgentOnChunk sets the agent-level OnChunk callback (merge semantics as WithAgentOnStepEnd).
func WithAgentOnChunk(fn func(ChunkEvent)) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.OnChunk = fn }
}

// WithAgentOnError sets the agent-level OnError callback (merge semantics as WithAgentOnStepEnd).
func WithAgentOnError(fn func(error)) AgentOption {
	return func(a *ToolLoopAgent) { a.settings.OnError = fn }
}

// mergeRequest builds the effective GenerateTextRequest for one Generate/
// Stream call: it starts from the agent's defaults (falling back to
// IsStepCount(20) when the agent set no StopWhen — ai-sdk-node's
// ToolLoopAgent default), applies opts in order so a call-level Option
// replaces the corresponding scalar field, then merges the agent-level and
// call-level lifecycle callbacks so both run instead of the call-level one
// silently discarding the agent's.
func (a *ToolLoopAgent) mergeRequest(opts []Option) GenerateTextRequest {
	req := a.settings
	if req.StopWhen == nil {
		req.StopWhen = IsStepCount(20)
	}

	agentOnStepEnd, agentOnEnd := req.OnStepEnd, req.OnEnd
	agentOnChunk, agentOnError := req.OnChunk, req.OnError
	req.OnStepEnd, req.OnEnd, req.OnChunk, req.OnError = nil, nil, nil, nil

	for _, o := range opts {
		o(&req)
	}

	req.OnStepEnd = mergeCallback(agentOnStepEnd, req.OnStepEnd)
	req.OnEnd = mergeCallback(agentOnEnd, req.OnEnd)
	req.OnChunk = mergeCallback(agentOnChunk, req.OnChunk)
	req.OnError = mergeCallback(agentOnError, req.OnError)

	return req
}

// mergeCallback combines an agent-level and a call-level callback of the same
// shape into one that invokes both — each on its own goroutine, recovered and
// logged independently — and waits for both before returning. This mirrors
// node's mergeCallbacks (util/merge-callbacks.ts), which runs callbacks under
// Promise.allSettled: concurrent, and neither callback's panic can affect the
// other or fail the run. When only one side is set, it is returned unchanged
// (no goroutine needed; the engine already recovers it as a plain observer).
//
// Note the concurrency is real parallelism, unlike node's single-threaded
// Promise.allSettled: the two callbacks run on separate goroutines, so if both
// mutate shared state they must synchronize it themselves.
func mergeCallback[T any](agent, call func(T)) func(T) {
	if agent == nil {
		return call
	}
	if call == nil {
		return agent
	}
	return func(ev T) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			defer safego.Recover(nil, nil, "callback", "agent-merge")
			agent(ev)
		}()
		go func() {
			defer wg.Done()
			defer safego.Recover(nil, nil, "callback", "call-merge")
			call(ev)
		}()
		wg.Wait()
	}
}
