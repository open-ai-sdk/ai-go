# Hooks

Hooks add synchronous, ordered lifecycle policy and observation to an Agent
run. Register defaults with `agent.Builder.Hook`, or append a run-only hook
with `Runner.Hook`. Agent hooks run before Runner hooks.

`HookFuncs` is the small function adapter. Implement a capability interface
directly when a reusable hook only needs one event.

```go
audit := agent.HookFuncs{
  Name: "audit-tools",
  BeforeToolFunc: func(ctx context.Context, hc agent.HookContext, call aikit.ToolCallInfo) (agent.ToolCallAction, error) {
    if call.Name == "delete_everything" {
      return agent.ToolCallAction{Kind: agent.ToolCallSkip, Reason: "not allowed"}, nil
    }
    return agent.ToolCallAction{Kind: agent.ToolCallRun}, nil
  },
}
```

Completion hooks can continue, patch the current request, or stop. Patches are
current-turn-only: later scalar settings and tool choice win, provider options
shallow-merge, and each active-tool patch intersects the currently active
definitions. Tool-call and tool-result rewrites chain in registration order;
skip and stop are terminal.

`HookContext` has a stable run ID, Agent ID, stream flag, one-based turn, and a
race-safe run-local scratchpad. Use `Store`, `Load`, `Update`,
`ScratchpadGet`, and `ScratchpadUpdate`; never place request credentials or
tool arguments in it.

Rich `ToolResultHook` callbacks receive immutable raw execution facts and a
separate mutable presentation. A presentation rewrite cannot change host-only
metadata or the execution result seen by later auditing code. Hooks are useful
guardrails and observability, not authorization: every tool must still enforce
its own authorization policy.

`Run` and `Stream` use the same lifecycle. Streaming observers receive the
same committed events as `Run`; cancellation stops further work. See
[Tools](/core/tools) for the tool result and error contracts.
