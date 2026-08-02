# Concepts

Concept pages explain the stable abstractions used across providers and
application workflows. They are organized by SDK capability rather than by
individual helper function.

- [Providers and clients](/core/providers-and-clients) — provider-wide state,
  model handles, capabilities, and mixed-modality output.
- [Agents](/core/agents) — reusable defaults and multi-step tool orchestration.
- [Completions](/core/completions) — provider-neutral language-model requests
  and responses.
- [Streaming](/core/streaming) — normalized model and step events.
- [Structured output](/core/structured-output) — schema-backed Go values.
- [Tools](/core/tools) — typed tools, execution, and approvals.
- [Embeddings](/core/embeddings) — single and batched vector generation.
- [Media generation](/core/media-generation) — image models, inputs, and
  generated media.
- [Observability](/core/observability) — provider-neutral tracing and content
  recording controls.

Provider setup and protocol adapters belong under
[Integrations](/integrations/). End-to-end application workflows belong under
[Tutorials & Guides](/guides/), and identifier-level details belong in the
[API reference](https://pkg.go.dev/github.com/open-ai-sdk/ai-go).
