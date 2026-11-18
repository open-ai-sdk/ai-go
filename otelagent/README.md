# otelagent

Optional OpenTelemetry adapter for `github.com/open-ai-sdk/ai-go/agent.Tracer`.

This is a regular package in the `github.com/open-ai-sdk/ai-go` module. Import it
only when the application wants to adapt an OpenTelemetry tracer to the core
runtime's provider-neutral tracing seam.

## Attributes

The agent runtime records two attribute families on each model-call span
(`ai.model_call`). The adapter maps `string`, `int`, `int64`, `float64`, `bool`
and `[]string` values to native OpenTelemetry attribute types and renders
anything else with `fmt.Sprint`.

Existing `ai.*` attributes are unchanged: `ai.step_number`, `ai.model_id`,
`ai.finish_reason`, `ai.usage.input_tokens`, `ai.usage.output_tokens`,
`ai.usage.total_tokens`.

OpenTelemetry generative-AI semantic-convention attributes, each emitted only
when the provider reported a value:

| Attribute | Source |
| --- | --- |
| `gen_ai.request.model` | the model ID the call was made with (set at span start) |
| `gen_ai.response.model` | `model` string in the finish event's provider metadata |
| `gen_ai.response.id` | `responseId` string in the finish event's provider metadata |
| `gen_ai.response.finish_reasons` | `string[]` with the provider's raw finish reason, or the normalized reason when none was reported |
| `gen_ai.usage.input_tokens` | `Usage.InputTokens` |
| `gen_ai.usage.output_tokens` | `Usage.OutputTokens` |
| `gen_ai.usage.cache_read.input_tokens` | `Usage.InputTokenDetails.CacheReadTokens`, when non-zero |
| `gen_ai.usage.cache_creation.input_tokens` | `Usage.InputTokenDetails.CacheWriteTokens`, when non-zero |
| `gen_ai.usage.reasoning.output_tokens` | `Usage.OutputTokenDetails.ReasoningTokens`, when non-zero |

The response id and model are read generically: the top level of the provider
metadata map is checked first, then every provider's nested map in sorted
provider-name order, so any provider that reports `responseId` / `model` is
picked up. The cache and reasoning keys follow the semantic conventions'
dedicated usage attributes; backends on an older convention version may not
recognize them but still store them as plain attributes.

Prompt and completion text are never recorded unless the runtime is built with
`WithTraceContent`.
