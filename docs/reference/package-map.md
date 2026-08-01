# Package map

The dependency graph points downward. `aikit` is the leaf package; providers do not import the UI protocol, and `aisdk` does not import agents or providers.

| Package | Responsibility |
| --- | --- |
| `ai` | Ergonomic facade for generation, streaming, objects, embeddings, and aliases |
| `agent/generate` | High-level generation, aggregation, request options, and stream results |
| `agent` | Multi-step loop, approval, stop conditions, callbacks, and step events |
| `aikit` | Dependency-free messages, events, usage, warnings, and errors |
| `llm` | Model contracts, request builder, provider options, embeddings, and images |
| `tool` | Typed/dynamic tools, schemas, registry, errors, and execution context |
| `transport` | Provider HTTP policy, retries, SSE reading, cancellation, and API error mapping |
| `provider/*` | Provider-specific constructors and wire codecs |
| `aisdk` | AI SDK v7 chunks, SSE framing, conversions, and approval signatures |
| `aisdkhttp` | HTTP request/SSE boundary for AI SDK v7 UI streams |
| `mcp` | MCP clients and dynamic tool integration |

The [README](https://github.com/open-ai-sdk/ai-go#readme) and Go package documentation remain the canonical API references. This site focuses on the stable workflows and architectural boundaries that make those APIs easier to compose.
