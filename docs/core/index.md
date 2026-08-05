# Core concepts

These pages describe stable contracts rather than one-off helpers. Read the
group matching the application you are building; the sidebar follows the same
order.

## Model foundations

- [Messages and content](/core/messages-and-content) — ordered conversations,
  roles, and multimodal parts.
- [Providers and clients](/core/providers-and-clients) — provider-wide state,
  model handles, capabilities, and mixed-modality output.
- [Completions](/core/completions) — provider-neutral language-model requests
  and responses.
- [Streaming](/core/streaming) — normalized model and Agent Runner events.

## Agents and tools

- [Agents](/core/agents) — reusable immutable model, tool, and policy defaults.
- [Agent Runner](/core/agent-runner) — ordered input, per-run overrides,
  multi-turn execution, and results.
- [Tools](/core/tools) — typed tools, rich results, safe errors, invocation
  context, and approvals.
- [Structured output](/core/structured-output) — schema-backed Go values.
- [Hooks](/core/hooks) — run-local lifecycle policy, retries, stream
  observation, and result presentation.

## Additional capabilities

- [Embeddings](/core/embeddings) — single and batched vector generation.
- [Media generation](/core/media-generation) — image models, inputs, and
  generated media.
- [Observability](/core/observability) — provider-neutral tracing and content
  recording controls.

Provider setup and protocol adapters belong under [Integrations](/integrations/).
For end-to-end workflows, use [Tutorials & Guides](/guides/); for package-level
details, use the [Go API reference](https://pkg.go.dev/github.com/open-ai-sdk/ai-go).
