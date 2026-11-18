# Providers

The `llm` contracts keep generation provider-neutral while each `provider/*`
package owns a provider's configuration, API encoding, and typed options.

For providers with the client architecture, construct one concrete client and
derive lightweight, operation-specific model handles from it. The client owns
credentials, endpoints, and reusable HTTP resources; its method set shows which
capabilities are implemented. The generic `provider.Client[P]` is reusable
infrastructure for provider authors rather than the normal application API.

| Provider | Package | Main capability |
| --- | --- | --- |
| OpenAI | `provider/openai` | Responses API, Chat Completions, and native Images API |
| Anthropic | `provider/anthropic` | Language models |
| Gemini | `provider/gemini` | Language, embeddings, images, and native Gemini features |
| OpenRouter | `provider/openrouter` | Chat, Responses and synchronous image models |
| Kie | `provider/kie` | [Language models](/providers/kie-language), images, video and jobs |
| Compatible endpoints | `provider/openaicompat` | OpenAI-style Chat Completions and embeddings |

Start with [OpenAI](/providers/openai), read the
[providers and clients concept guide](/core/providers-and-clients), or see
[other providers](/providers/other-providers). Provider options should normally
be typed structs from that provider package. This catches unsupported or invalid
options rather than silently ignoring them.
