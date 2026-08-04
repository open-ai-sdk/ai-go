# Structured output

Structured output turns a model response into a Go value. ai-go derives a
strict JSON Schema, finds the first complete JSON value in the response,
validates it locally, then decodes it into the requested type. Markdown fences
and surrounding prose are accepted; invalid JSON and schema-invalid values are
reported as typed errors.

This page follows the Extractors information architecture from Rig, but only
describes ai-go behavior that is implemented today.

## Pick a surface

| Surface | Best for | Returns |
| --- | --- | --- |
| `agent.RunObject[T]` | One Agent/Runner invocation | `ObjectResult[T]`, including the full `Result` |
| `extractor.New[T](model).Build()` | Repeated extraction using one prepared schema | `extractor.Result[T]` |

Use `ai.NewTypedCompletion[T]` for a simple request/response boundary; it is
documented with [Completions](/core/completions), not as an extraction API. Use
`RunObject` when the surrounding Agent lifecycle, transcript, hooks, tools, or
step usage matter. Use `extractor.New[T]` when many inputs share the same
target type and configuration.

## Core concepts

Every structured-output surface uses one parse → validate → decode pipeline.
The schema is derived once for `RunObject`; an `extractor.Extractor` retains it
for its lifetime. `RunObject` replaces a schema previously configured on its
Runner, so the Go type is the source of truth.

`OutputMode` has `Auto`, `Native`, and `Tool` values. `Auto` resolves from the
model capability before provider I/O. Native mode is the supported production
path. The public Tool-mode resolver exists, but agent-level synthetic output
tool interception is not implemented yet; do not force `OutputModeTool` in an
application.

## Provider capability matrix

| Provider/model path | Native schema | With ordinary tools | Notes |
| --- | --- | --- | --- |
| OpenAI Responses | Yes | Yes | Uses `text.format` JSON Schema |
| OpenAI Chat | Yes | Yes | Uses `response_format` JSON Schema |
| Gemini OpenAI-compatible | Yes | Yes | Output schema is normalized for Gemini |
| Gemini native | Yes | No | Native API rejects schema plus function declarations |
| Anthropic supported Claude 4.1/4.5+ IDs | Yes | Yes | Uses `output_config.format` |
| Unknown or older Anthropic IDs | No | — | Fails locally with a structured-output error |
| Generic OpenAI-compatible model | Configured by provider | Depends on provider | Set the provider capability explicitly |

## Target type requirements

`tool.StrictSchema[T]` supports exported struct fields, nested structs, arrays,
numeric and boolean scalars, strings, and `enum` tags. Recursive types, maps,
interfaces, and custom JSON decoders are rejected because they cannot produce a
portable strict schema.

Every generated object has `additionalProperties: false`. Every property is
listed in `required`; Go optionality is represented by accepting `null`:

| Go field | Strict-schema behavior |
| --- | --- |
| `Name string` | Required string |
| `Name *string` | Required, string or null |
| `Name string` with `omitempty` | Required, string or null |
| Optional `enum:"a,b"` string | Required, `a`, `b`, or null |

`tool.Schema[T]` is intentionally unchanged. It remains the schema used for
tool inputs; use `StrictSchema[T]` only for output constraints.

## Usage

Runnable versions are in the sibling playground: run
`go run ./examples/05-02-run-object`,
`go run ./examples/05-03-extractor`,
`go run ./examples/05-04-extractor-context`,
`go run ./examples/05-05-extractor-errors`, or
`go run ./examples/05-06-extractor-batch` from `test-ai-go`.

- Runner: build an `agent.Agent`, create a Runner with a prompt, then call
  `RunObject[T]`.
- Extractor: call `extractor.New[T](model)`, configure it with value-builder
  methods, then call `Build` once and reuse `Extract` or `ExtractWithUsage`.

## Extractor configuration and reuse

The `extractor` builder has `Instructions`, `Context`, `Settings`,
`ProviderOptions`, and `Retries`. Settings and provider options are applied to
every attempt. `ExtractWithHistory` accepts caller-owned history plus the new
input; it does not mutate that slice.

An extractor requires only decode-side compatibility from `T`; there is no
encode-side constraint. It is not an Agent tool and must not be registered in a
`tool.Set`.

## Errors and validation

`llm.StructuredOutputError` is also re-exported as
`agent.StructuredOutputError` and `ai.StructuredOutputError`. Its kinds are:

| Kind | Meaning |
| --- | --- |
| `prompt` | Provider request could not be made or model capability is unsupported |
| `empty` | The provider returned no usable text |
| `json_decode` | No complete JSON value could be decoded |
| `validation` | JSON does not satisfy the output schema |

Use `errors.Is` for a kind and `errors.As` to inspect the error. `Extractor`
returns `ExtractionError` after all attempts fail; it contains the final kind,
attempt count, accumulated usage, and underlying cause. It retries output
failures only, defaults to zero retries, and never retries a cancelled context
or a prompt/provider failure.

## Agent results, streaming, and UI

For a normal final text turn, the Agent parses the existing response instead of
issuing a dedicated finishing call. `MaxTurns: 1` is therefore sufficient for a
tool-free structured response. A terminal tool turn can still need one
constrained finishing call; that JSON is emitted only as
`StepEventStructuredOutput`, never as `Result.Text` or a transcript message.

Structured output is server-side only today. `aisdk` does not map
`StepEventStructuredOutput` to a `useChat` data chunk, and partial objects are
not streamed. Consume `Result.StructuredOutput` or the server-side event.

## Best practices

- Keep target types narrow and document fields with Go tags.
- Prefer nullable fields over relying on a field being omitted.
- Handle `ExtractionError` usage when retries are enabled: every attempt may be
  billed.
- Preserve the returned `Result` or direct response for diagnostics, but do not
  log raw provider text without applying your data-handling policy.
- Choose a provider/model from the capability matrix before combining output
  constraints with tools.

## Common patterns and troubleshooting

For classification or batch work, build one `extractor.Extractor[T]` and call it
for each document. For a single agent workflow, use `RunObject[T]` so
transcript and tool results remain available beside the decoded value. For a
single completion, use `ai.NewTypedCompletion[T]`.

If a result fails validation, first inspect the typed error kind and the
provider response or Agent Result. An `empty` error usually means the model did
not produce content; `json_decode` means no JSON value was present; and
`validation` means the value did not satisfy the generated schema. A prompt
error for Anthropic commonly means the selected model ID lacks native
structured-output support.

## See also

- [Completions](/core/completions)
- [Agent Runner](/core/agent-runner)
- [Error handling](/guides/error-handling)
