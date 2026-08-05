# Integrations

Integrations connect ai-go's provider-neutral contracts to a concrete remote
service or browser client. Start with the group your application needs; each
page owns configuration and wire-protocol specifics.

## Model providers

Concrete `provider/*` packages own authentication, endpoints, wire formats, and
typed options. Start from the [provider overview](/providers/), then choose
[OpenAI](/providers/openai) or [another provider](/providers/other-providers).

## Agent and tool protocols

[Model Context Protocol](/integrations/mcp) adapts MCP capabilities into tools
that the Agent runtime can call.

## UI streams

[UI streams](/integrations/uistream) is the shared `uistream` subsystem for
browser-facing event streams. From its hub, choose [AI SDK v7 UI streams](/integrations/ui-streams)
for `useChat`, [AG-UI and TanStack AI](/integrations/ag-ui) for TanStack, or
the shared protocol seam for a custom client.

## Build an integration

Use [Extension points](/docs/extensions) for a new model or provider. For a
browser protocol, begin at the [UI streams hub](/integrations/uistream).

For the shared provider/client mental model, see
[Providers and clients](/core/providers-and-clients).
