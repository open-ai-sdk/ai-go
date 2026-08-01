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
      text: View on GitHub
      link: https://github.com/open-ai-sdk/ai-go

features:
  - icon: ⚡
    title: One ergonomic facade
    details: Generate text and objects, stream responses, and embed content through the top-level ai package.
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

Install the module, configure a provider, then call `ai.GenerateText`. The facade keeps the common path small while lower packages retain explicit ownership of contracts and protocols.

```sh
go get github.com/open-ai-sdk/ai-go
```

[Read the quickstart →](/getting-started)
