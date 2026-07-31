# AI/engine type-duplication inventory

Phase 02 resolves the 23 concepts that previously crossed the `ai` and
`internal/engine` boundary through mirrored structs or callback adapters.
“Identical” means the declarations had the same fields and semantics;
“superset” identifies the shape retained in `aikit`.

| # | `ai/` declaration | Former `internal/engine/` declaration | Classification | Resolution |
|---:|---|---|---|---|
| 1 | `LanguageModel` | `Model` | identical contract, different request names | `aikit.Model`; both layers name the same interface |
| 2 | `Message` | `Message` | ai-superset (`Role`) | `aikit.Message` retains the typed role |
| 3 | `ContentPart` | `ContentPart` | ai-superset (`json.RawMessage`, distinct reasoning field) | `aikit.ContentPart`; engine history uses the public representation |
| 4 | `ToolChoice` | `ToolChoice` | identical | `aikit.ToolChoice` |
| 5 | `LanguageModelRequest` | `Request` | engine-superset (`ToolsContext`, `RuntimeContext`) | `aikit.ModelRequest` contains every field |
| 6 | `OutputSchema` | `OutputSchema` | identical | `aikit.OutputSchema` |
| 7 | `CallSettings` | `CallSettings` | identical | `aikit.CallSettings` |
| 8 | `StopCondition` | `StopCondition` | identical after `StepResult` unification | `aikit.StopCondition` |
| 9 | `StepResult` | `StepResult` | identical | `aikit.StepResult` |
| 10 | `PrepareStepContext` | `PrepareStepContext` | engine-superset step history | `aikit.PrepareStepContext` exposes the complete history |
| 11 | `PrepareStepInfo` | `StepResultInfo` | engine-superset | `aikit.PrepareStepInfo` retains reasoning, calls, results, usage, raw finish reason, metadata, and warnings |
| 12 | `PrepareStepResult` | `PrepareStepResult` | identical after model/tool unification | `aikit.PrepareStepResult` |
| 13 | `PrepareStepFunc` | `PrepareStepFunc` | identical | `aikit.PrepareStepFunc` |
| 14 | `StepEndEvent` | `StepEndEvent` | genuinely different | Kept separate: the public event adds the SDK-specific continuation `Response`; the engine event is emitted before that response is assembled |
| 15 | `EndEvent` | `EndEvent` | genuinely different | Kept separate: public `Steps` are final `StepOutput` values with continuation responses; engine steps are prepare-step history |
| 16 | `ToolCallOutput` | `ToolCallInfo` | engine-superset plus representation mismatch | `aikit.ToolCallInfo` uses `json.RawMessage` and retains `ArgsSet` for repair semantics |
| 17 | `RepairToolCallInput` | `ToolCallRepairContext` | identical after message/tool-call unification | `aikit.RepairToolCallInput` |
| 18 | `RepairToolCallFunc` | `ToolCallRepairFunc` | identical | `aikit.RepairToolCallFunc` |
| 19 | `ToolExecutor` | `ToolExecutor` | identical | `aikit.ToolExecutor` |
| 20 | `ToolDefinition` | `ToolDefinition` | identical | `aikit.ToolDefinition` |
| 21 | `ToolSet` | `ToolSet` | identical, including duplicated index behavior | `aikit.ToolSet`; the engine copy and its three index helpers were deleted |
| 22 | callback fields on `GenerateTextRequest` | `LifecycleCallbacks` | genuinely different | Kept separate: the public request owns four optional callbacks, while the engine needs an execution-time bundle whose step/end events precede response enrichment |
| 23 | `GenerateTextRequest` | `RunParams` | genuinely different | Kept separate: smoothing/middleware are façade concerns; tracing, logging, approval responder, and contextual execution are engine wiring. `runParams` maps only these execution concerns and performs no vocabulary conversion |

## Outcome

- Shared concepts are declared once in dependency-free `aikit`.
- `ai` keeps source-facing aliases and constructors; it no longer rebuilds
  messages, content parts, model requests, tools, usage, warnings, sources, or
  stream events field by field.
- The four deliberately distinct execution/facade concepts are documented
  above rather than disguised as near-identical shared structs.
