# Extend ai-go

ai-go exposes extension points at different layers. Choose the smallest one
that matches the integration.

## Implement a model contract

Implement `llm.Model` for a language model, `llm.EmbeddingModel` for embeddings,
or `llm.ImageModel` for image generation. These interfaces are intentionally
small so external packages can implement and test them without depending on an
agent runtime.

## Build a provider client

Provider authors can compose `provider.Client[P]` with a policy that supplies a
provider name, base URL, and request authorization. The concrete provider
package should then expose a concrete Client whose method set contains only the
operations it implements.

Read [Providers and clients](/core/providers-and-clients) for the ownership and
capability model.

## Adapt an OpenAI-compatible endpoint

The `provider/openaicompat` package provides the Chat Completions protocol plus
optional hooks for provider naming, capability flags, tool sanitization,
request rewriting, and response decoding. Use it when the remote API follows
the OpenAI-compatible wire shape; implement a native model when it does not.

See the [provider integrations](/providers/) and the
[Go API reference](https://pkg.go.dev/github.com/open-ai-sdk/ai-go/provider/openaicompat)
for the exact contracts.
