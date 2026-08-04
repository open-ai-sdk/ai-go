# Concepts

Concept pages explain the stable abstractions used across providers and
application workflows. They are organized by SDK capability rather than by
individual helper function.

- [Providers and clients](/core/providers-and-clients) — provider-wide state,
  model handles, capabilities, and mixed-modality output.
- [Completions](/core/completions) — provider-neutral language-model requests
  and responses.
- [Tools](/core/tools) — typed tools, execution, and approvals.
- [Agents](/core/agents) — reusable immutable model, tool, and policy defaults.
- [Agent Runner](/core/agent-runner) — ordered input, per-run overrides,
  multi-turn execution, results, and hooks.
- [Streaming](/core/streaming) — normalized model and Agent Runner events.
- [Structured output](/core/structured-output) — schema-backed Go values.
- [Embeddings](/core/embeddings) — single and batched vector generation.
- [Media generation](/core/media-generation) — image models, inputs, and
  generated media.
- [Observability](/core/observability) — provider-neutral tracing and content
  recording controls.

Provider setup and protocol adapters belong under
[Integrations](/integrations/). End-to-end application workflows belong under
[Tutorials & Guides](/guides/), and identifier-level details belong in the
[API reference](https://pkg.go.dev/github.com/open-ai-sdk/ai-go).
