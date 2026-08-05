---
layout: home

hero:
  name: ai-go
  text: Provider-neutral AI application primitives for Go
  tagline: Model calls, typed tools, multi-step agents, and browser UI streams from focused Go packages.
  actions:
    - theme: brand
      text: Get started
      link: /getting-started
    - theme: alt
      text: Explore the docs
      link: /docs/

features:
  - icon: ⚡
    title: Model foundations
    details: Keep messages, completions, streaming, embeddings, and media behind small provider-neutral contracts.
  - icon: 🧰
    title: Agents and typed tools
    details: Build an immutable Agent once, create a Runner per invocation, and let the runtime execute tool loops.
  - icon: 🔌
    title: Concrete integrations
    details: Use focused provider clients, MCP tools, and explicit extension points instead of a single monolithic abstraction.
  - icon: 🌊
    title: One UI-stream boundary
    details: Serve AI SDK v7 and AG-UI from the shared uistream lifecycle, or add a protocol adapter of your own.
---

ai-go is heavily inspired by [Rig](https://rig.rs/), adapted to Go's type
system, explicit package ownership, and standard library conventions.

## Choose a path

### Make your first model call

Install ai-go and configure a provider in [Get started](/getting-started). Then
learn [Providers and clients](/core/providers-and-clients) and
[Completions](/core/completions).

```sh
go get github.com/open-ai-sdk/ai-go
```

### Build an agent workflow

Start with [Agents](/core/agents), create a per-request
[Agent Runner](/core/agent-runner), and add [Tools](/core/tools) when the model
must call application code.

### Connect a client or service

Browse [Integrations](/integrations/) for providers and MCP. For browser
responses, begin at [UI streams](/integrations/uistream): it groups the AI SDK
v7, AG-UI, and custom-adapter documentation around one shared boundary.

### Find a focused answer

[Documentation](/docs/) follows the recommended reading order. Use
[Tutorials & Guides](/guides/) for end-to-end workflows,
[Examples](/examples/) for runnable source, and the
[package map](/reference/package-map) or
[Go API reference](https://pkg.go.dev/github.com/open-ai-sdk/ai-go) for exact
package ownership and identifiers.
