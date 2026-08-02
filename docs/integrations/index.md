# Integrations

Integrations connect ai-go's model and event contracts to external services and
protocols.

## Model providers

Use concrete packages under `provider/*` for authentication, endpoints, wire
formats, and typed provider options.

- [Model provider overview](/providers/)
- [OpenAI](/providers/openai)
- [Other providers](/providers/other-providers)

## Protocol integrations

- [Model Context Protocol](/integrations/mcp) turns MCP capabilities into tools
  that the agent runtime can call.
- [AI SDK v7 UI streams](/integrations/ui-streams) translates normalized events
  into the browser-facing SSE protocol.

For the provider/client mental model, start with
[Providers and clients](/core/providers-and-clients).
