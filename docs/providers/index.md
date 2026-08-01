# Providers

The `llm` contracts keep generation provider-neutral while each `provider/*` package owns a provider's constructor, API encoding, and typed options.

| Provider | Package | Main capability |
| --- | --- | --- |
| OpenAI | `provider/openai` | Responses API and Chat Completions |
| Anthropic | `provider/anthropic` | Language models |
| Gemini | `provider/gemini` | Language, embeddings, images, and native Gemini features |
| Kie | `provider/kie` | Image models |
| Compatible endpoints | `provider/openaicompat` | OpenAI-style Chat Completions |

Start with [OpenAI](/providers/openai) or see [other providers](/providers/other-providers). Provider options should normally be typed structs from that provider package. This catches unsupported or invalid options rather than silently ignoring them.
