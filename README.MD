# ai-go

`ai-go` is a provider-neutral Go SDK for direct model calls, typed tools,
multi-step agents, and AI SDK v7 UI streams.

## Documentation

Start at [open-ai-sdk.github.io/ai-go](https://open-ai-sdk.github.io/ai-go/).
It contains the quickstart, guides, provider setup, concepts, integrations,
and API-oriented documentation.

- [Quickstart](https://open-ai-sdk.github.io/ai-go/getting-started)
- [Agents](https://open-ai-sdk.github.io/ai-go/core/agents)
- [Tools](https://open-ai-sdk.github.io/ai-go/core/tools)
- [Hooks](https://open-ai-sdk.github.io/ai-go/core/hooks)
- [Go API reference](https://pkg.go.dev/github.com/open-ai-sdk/ai-go)

## Install

```sh
go get github.com/open-ai-sdk/ai-go
```

Requires Go 1.25 or newer.

## API at a glance

| Need | API |
| --- | --- |
| One direct model call | `llm.NewCompletion(model, prompt).Send(ctx)` |
| A reusable multi-step agent | `agent.New(model).Tools(tools).Build()` |
| A per-request execution | `assistant.Runner().Prompt(prompt).Run(ctx)` |
| A function-based tool | `tool.New(name, description, handler)` |
| A struct-based tool | `tool.Adapt(typedTool)` |
| An immutable tool registry | `tool.NewSet(tools...)` |
| AI SDK v7 HTTP streaming | `aisdkhttp.Handler(run)` |

The module is pre-1.0. Review the [changelog](CHANGELOG.md) before upgrading.

## Inspiration

ai-go is heavily inspired by [Rig](https://rig.rs/), adapted to Go's type
system, explicit package ownership, and standard library conventions.

## License

MIT. See [LICENSE](LICENSE).
