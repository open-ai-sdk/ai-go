# Option surface matrix

Phase 03 inventory of the 27 `GenerateTextRequest` fields plus the six
`CallSettings` fields. `RequestBuilder` covers every row. The narrower
`llm.RequestBuilder` covers only values that cross the model boundary; tool-loop
execution values intentionally remain in `ai`.

| # | Knob | Explicit struct | `ai.RequestBuilder` | `ai.Option` | `AgentOption` | `llm.RequestBuilder` |
|---:|---|:---:|:---:|:---:|:---:|:---:|
| 1 | Model | ✓ | `Model` | `WithModel` | constructor | — |
| 2 | Instructions | ✓ | `Instructions` | `WithInstructions` | `WithAgentInstructions` | `Instructions` |
| 3 | Messages | ✓ | `Messages` | `WithMessages` | call only | `Messages`/`Prompt` |
| 4 | Tools | ✓ | `Tools` | `WithTools` | `WithAgentTools` | `Tools` |
| 5 | ToolChoice | ✓ | `ToolChoice` | `WithToolChoice` | `WithAgentToolChoice` | `ToolChoice` |
| 6 | StopWhen | ✓ | `StopWhen` | `WithStopWhen` | `WithAgentStopWhen` | — |
| 7 | Output | ✓ | `Output` | `WithOutput` | `WithAgentOutput` | `Output` |
| 8 | Settings (aggregate) | ✓ | `Settings` | — | — | `Settings` |
| 9 | MaxSteps | ✓ | `MaxSteps` | `WithMaxSteps` | — | — |
| 10 | ProviderOptions | ✓ | `With`/`ProviderOptionsJSON` | `WithProviderOptions` | `WithAgentProviderOptions` | `With`/`ProviderOptionsJSON` |
| 11 | PrepareStep | ✓ | `PrepareStep` | `WithPrepareStep` | `WithAgentPrepareStep` | — |
| 12 | RepairToolCall | ✓ | `RepairToolCall` | `WithRepairToolCall` | `WithAgentRepairToolCall` | — |
| 13 | ActiveTools | ✓ | `ActiveTools` | `WithActiveTools` | — | — |
| 14 | ToolsContext | ✓ | `ToolsContext` | `WithToolsContext` | — | `ToolsContext` |
| 15 | RuntimeContext | ✓ | `RuntimeContext` | `WithRuntimeContext` | — | `RuntimeContext` |
| 16 | ToolApproval | ✓ | `ToolApproval` | `WithToolApproval` | `WithAgentToolApproval` | — |
| 17 | ToolApprovalResponder | ✓ | `ToolApprovalResponder` | — | `WithAgentToolApprovalResponder` | — |
| 18 | OnStepEnd | ✓ | `OnStepEnd` | `WithOnStepEnd` | `WithAgentOnStepEnd` | — |
| 19 | OnEnd | ✓ | `OnEnd` | `WithOnEnd` | `WithAgentOnEnd` | — |
| 20 | OnChunk | ✓ | `OnChunk` | `WithOnChunk` | `WithAgentOnChunk` | — |
| 21 | OnError | ✓ | `OnError` | `WithOnError` | `WithAgentOnError` | — |
| 22 | SmoothStream | ✓ | `SmoothStream` | `WithSmoothStream` | — | — |
| 23 | Middlewares | ✓ | `Middlewares` | `WithMiddleware`/retry options | — | — |
| 24 | ParallelToolExecution | ✓ | `ParallelToolExecution` | `WithParallelToolExecution` | `WithAgentParallelToolExecution` | — |
| 25 | MaxParallelTools | ✓ | `MaxParallelTools` | `WithMaxParallelTools` | — | — |
| 26 | Logger | ✓ | `Logger` | `WithLogger` | — | — |
| 27 | TraceContent | ✓ | `TraceContent` | `WithTraceContent` | — | — |
| 28 | Temperature | ✓ | `Temperature` | `WithTemperature` | — | `Temperature` |
| 29 | MaxTokens | ✓ | `MaxTokens` | `WithMaxTokens` | — | `MaxTokens` |
| 30 | TopP | ✓ | `TopP` | `WithTopP` | — | `TopP` |
| 31 | TopK | ✓ | `TopK` | `WithTopK` | — | `TopK` |
| 32 | Seed | ✓ | `Seed` | `WithSeed` | — | `Seed` |
| 33 | StopSequences | ✓ | `StopSequences` | `WithStopSequences` | — | `StopSequences` |

The old option families remain callable for source compatibility during the
package restructure. New examples use `RequestBuilder`; Phase 07 can make agent
defaults consume the same builder without moving agent concerns into `llm`.
