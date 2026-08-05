# Documentation

ai-go is arranged around a small set of stable contracts. Read this site in the
same order you build an application: start a model call, learn the core
abstractions, connect an integration, then reach for a guide or example.

## 1. Start here

- [Get started](/getting-started) installs ai-go and makes one provider-backed
  model call.
- [Why ai-go](/docs/why-ai-go) explains the package and compatibility goals.
- [Architecture](/docs/architecture) shows dependency direction and public
  ownership.

## 2. Learn the core concepts

[Core concepts](/core/) groups the contracts by responsibility:

- model input and output: [Messages](/core/messages-and-content),
  [Providers and clients](/core/providers-and-clients),
  [Completions](/core/completions), and [Streaming](/core/streaming);
- reusable agent workflows: [Agents](/core/agents),
  [Agent Runner](/core/agent-runner), [Tools](/core/tools), and
  [Hooks](/core/hooks);
- additional capabilities: [Structured output](/core/structured-output),
  [Embeddings](/core/embeddings), [Media generation](/core/media-generation),
  and [Observability](/core/observability).

## 3. Connect services and clients

[Integrations](/integrations/) is where concrete external contracts live:
[model providers](/providers/), [MCP](/integrations/mcp), and the shared
[UI streams](/integrations/uistream) subsystem. Use [Extension points](/docs/extensions)
when the existing integrations do not match your service.

## 4. Build an application

[Tutorials & Guides](/guides/) combine several concepts into one workflow;
[Examples](/examples/) link to runnable repository programs. For exported
identifiers and package ownership, use the [package map](/reference/package-map)
and [Go API reference](https://pkg.go.dev/github.com/open-ai-sdk/ai-go).
