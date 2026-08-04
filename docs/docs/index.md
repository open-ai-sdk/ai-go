# Documentation

ai-go separates provider integrations, model contracts, agent workflows, and
transport protocols into focused Go packages. Start with the quickstart, then
use concepts to understand the abstractions and integrations to configure a
specific service.

## Quickstart

- [Get started](/getting-started) — install ai-go, create a provider client,
  and generate text.
- [Tools](/core/tools) — define schema-backed tools and execute multi-step
  model calls.
- [Agents](/core/agents) — configure reusable immutable model, tool, and policy
  defaults.
- [Agent Runner](/core/agent-runner) — add ordered input and request-local
  overrides, then execute or stream one invocation.

## Understand the architecture

- [Why ai-go](/docs/why-ai-go) explains the design goals and package boundaries.
- [Architecture](/docs/architecture) maps the public layers and dependency
  direction.
- [Concepts](/core/) covers providers, completions, messages, tools, Agents,
  Agent Runner, streaming, and structured output in dependency order.

## Connect services

- [Integrations](/integrations/) covers model providers, MCP, and AI SDK UI
  streams.
- [Extend ai-go](/docs/extensions) explains the existing extension seams for
  custom models and OpenAI-compatible services.

For runnable walkthroughs, continue to [Tutorials & Guides](/guides/) or browse
the [Examples](/examples/). Exact exported identifiers live in the
[Go API reference](https://pkg.go.dev/github.com/open-ai-sdk/ai-go).
