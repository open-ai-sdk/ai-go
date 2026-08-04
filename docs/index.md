---
layout: home

hero:
  name: ai-go
  text: AI application primitives for Go
  tagline: Provider-neutral generation, typed tools, multi-step agents, and AI SDK v7 UI streams.
  actions:
    - theme: brand
      text: Get started
      link: /getting-started
    - theme: alt
      text: Read the docs
      link: /docs/

features:
  - icon: ⚡
    title: Focused Go APIs
    details: Use llm for direct model calls, then build an immutable Agent and create a fresh Runner for each invocation.
  - icon: 🧰
    title: Typed tools and agents
    details: Build schema-validated Go tools and let the runtime execute multi-step tool loops.
  - icon: 🔌
    title: Provider-neutral by design
    details: Use OpenAI, Anthropic, Gemini, Kie, or OpenAI-compatible models behind focused contracts.
  - icon: 🌊
    title: UI-stream ready
    details: Produce AI SDK v7-compatible SSE streams from a small net/http boundary.
---

## Start with a model

Install the module, configure a provider, then use `llm.NewCompletion` for one
model call or `agent.New(...).Build()` for reusable multi-turn execution.
Canonical packages keep contract ownership explicit without Agent aliases or
compatibility shims.

Read [Agents](/core/agents) before [Agent Runner](/core/agent-runner): the first
defines reusable configuration, while the second owns input, overrides, and
execution.

```sh
go get github.com/open-ai-sdk/ai-go
```

[Read the quickstart →](/getting-started) · [Browse the documentation →](/docs/)
