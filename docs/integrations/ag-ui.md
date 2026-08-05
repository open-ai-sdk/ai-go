# AG-UI and TanStack AI

`uistream/agui` serves the [AG-UI](https://docs.ag-ui.com/) event stream that
[TanStack AI](https://tanstack.com/ai) clients consume. TanStack's `StreamChunk`
union *is* the AG-UI event union, so any `@tanstack/ai-react`,
`@tanstack/ai-vue`, `@tanstack/ai-svelte`, or `@tanstack/ai-solid` client can
talk to an ai-go server without a shim.

The adapter plugs into the same `uistream` driver as the AI SDK v7 adapter, so
one Agent can serve both protocols from one process.

## Serve the endpoint

```go
package main

import (
	"context"
	"iter"
	"log"
	"net/http"

	"github.com/open-ai-sdk/ai-go/agent"
	"github.com/open-ai-sdk/ai-go/aikit"
	"github.com/open-ai-sdk/ai-go/aisdkhttp"
	"github.com/open-ai-sdk/ai-go/uistream/agui"
)

func main() {
	assistant, err := agent.New(model).Build()
	if err != nil {
		log.Fatal(err)
	}

	run := func(ctx context.Context, messages []aikit.Message) (iter.Seq2[aikit.StepEvent, error], error) {
		return assistant.Runner().Messages(messages...).Stream(ctx)
	}

	// Handler defaults to AI SDK v7; HandlerFor selects a protocol.
	http.Handle("/ag-ui", aisdkhttp.HandlerFor(agui.Protocol(), run))
	http.Handle("/chat", aisdkhttp.Handler(run))

	log.Fatal(http.ListenAndServe(":8787", nil))
}
```

## Consume it

```tsx
import { fetchServerSentEvents } from '@tanstack/ai-react'
import { Chat, ChatInput, ChatMessage, ChatMessages } from '@tanstack/ai-react-ui'
import type { UIMessage } from '@tanstack/ai-react'

export function App() {
  const connection = fetchServerSentEvents('/ag-ui')

  return (
    <Chat connection={connection}>
      <ChatMessages>
        {(message: UIMessage) => <ChatMessage message={message} />}
      </ChatMessages>
      <ChatInput />
    </Chat>
  )
}
```

`fetchServerSentEvents(url, () => ({ body }))` merges `body` into the request's
`forwardedProps`, which the decoder exposes through `uistream.Request.Extra`.

## Request contract

The decoder accepts AG-UI's `RunAgentInput`:

| Field | Use |
| --- | --- |
| `threadId`, `runId` | required; echoed on every lifecycle event |
| `messages` | converted to `[]aikit.Message` and passed to the Runner |
| `state` | echoed as `STATE_SNAPSHOT` at an interrupt boundary |
| `resume` | client decisions resolving pending interrupts |
| `parentRunId`, `tools`, `context`, `forwardedProps` | surfaced on `Request.Extra` |

`aisdkhttp.HandlerFor` hands the run only the messages. To read anything else,
use `aisdkhttp.HandlerForRequest`, which passes the whole `uistream.Request`:

```go
mux.Handle("/ag-ui", aisdkhttp.HandlerForRequest(agui.Protocol(),
    func(ctx context.Context, request uistream.Request) (iter.Seq2[aikit.StepEvent, error], error) {
        entries, _ := request.Extra["resume"].([]agui.ResumeEntry)
        props, _ := request.Extra["forwardedProps"].(map[string]any)
        // …
    }))
```

Note what the client actually populates. `context` and `state` are hardcoded to
`[]` and `{}` by TanStack's request builder, so application data reaches the
server through **`forwardedProps`** alone. `forwardedProps` is entirely
client-controlled: treat it as untrusted input and never as authorization — a
production server authenticates an approval through the signed `resume` binding,
not through props.

`Extra["tools"]` is raw client JSON describing tools the *browser* executes. A
server must not register them into its own executable `tool.Set`.

TanStack fans one assistant turn out into several wire rows: `role: "reasoning"`
for thinking parts, the anchor `role: "assistant"` message, and one
`role: "tool"` row per tool result. Reasoning and `activity` rows carry no
history of their own and are dropped rather than rejected — the anchor message
already holds that turn's content.

## Emitted events

| Engine event | AG-UI events |
| --- | --- |
| run start / end | `RUN_STARTED`, `RUN_FINISHED`, `RUN_ERROR` |
| `StepEventStepStart` / `StepEventStepEnd` | `STEP_STARTED`, `STEP_FINISHED` — opt-in, see below |
| `StepEventTextDelta` | `TEXT_MESSAGE_START`, `TEXT_MESSAGE_CONTENT`, `TEXT_MESSAGE_END` |
| `StepEventReasoningDelta` | `REASONING_START`, `REASONING_MESSAGE_START`, `REASONING_MESSAGE_CONTENT`, `REASONING_MESSAGE_END`, `REASONING_END` |
| `StepEventToolCallStart` / `Delta` / `Ready` | `TOOL_CALL_START`, `TOOL_CALL_ARGS`, `TOOL_CALL_END` |
| `StepEventToolResult`, `ToolCallInvalid`, `ToolOutputDenied` | `TOOL_CALL_RESULT` |
| `StepEventToolApprovalRequest` | `MESSAGES_SNAPSHOT` + `RUN_FINISHED` with an interrupt outcome |
| `StepEventClientToolRequest` | `MESSAGES_SNAPSHOT` + `RUN_FINISHED` with a client-tool interrupt outcome |
| `StepEventSource` | `CUSTOM` named `source` |
| `StepEventFileDelta` | `CUSTOM` named `file` |
| `StepEventStructuredOutput` | `CUSTOM` named `structured-output.complete` |
| `StepEventStateSnapshot` | `STATE_SNAPSHOT` |
| `StepEventStateDelta` | `STATE_DELTA` |
| `StepEventUsage` | folded into `RUN_FINISHED.usage` |

### Reasoning uses `REASONING_*`, not `THINKING_*`

`THINKING_START` and its siblings are marked deprecated in `@ag-ui/core` and are
absent from TanStack's `StreamChunk` union, so its stream processor has no
handler for them. Emitting them would render nothing. `REASONING_MESSAGE_CONTENT`
is the event the processor actually consumes.

### `STEP_*` markers are off by default

TanStack AI overloads `STEP_STARTED` and `STEP_FINISHED` as its *reasoning*
transport — its own adapters carry thinking text in `STEP_FINISHED.delta`. Its
stream processor therefore builds a thinking part from any `STEP_FINISHED`,
**even one carrying no content**, so plain agent step boundaries render an empty
"thinking" block on every step.

ai-go already carries reasoning on `REASONING_*`, which the processor prefers
and de-duplicates against, so bare step markers are pure noise for this client
and are suppressed. Usage folding and per-step tool-state resets still happen —
only the two wire events are withheld.

Enable them for AG-UI clients that read step boundaries literally:

```go
agui.Protocol(agui.WithStepEvents())
```

### There is no `[DONE]` sentinel

`RUN_FINISHED` terminates an AG-UI stream. TanStack's SSE parser treats a
trailing `data: [DONE]` as a deprecated server and logs a warning. The AI SDK v7
endpoint still emits `[DONE]`, because there it *is* the protocol — the two
adapters differ on purpose.

### Token usage

AG-UI defines no usage field. TanStack reads `RUN_FINISHED.usage` in its
`TokenUsage` shape, so `aikit.Usage` is translated on the way out:

```json
{
  "type": "RUN_FINISHED",
  "threadId": "t1",
  "runId": "r1",
  "finishReason": "stop",
  "usage": { "promptTokens": 11, "completionTokens": 22, "totalTokens": 33 }
}
```

`RUN_FINISHED.finishReason` is a TanStack extension carried through
`@ag-ui/core`'s passthrough schema; it is omitted when the engine reports a
reason with no AG-UI equivalent.

### Errors

Terminal errors become `RUN_ERROR` with a redacted message and the run's
identity:

```json
{"type":"RUN_ERROR","message":"provider error (status 503)","threadId":"t1","runId":"r1"}
```

`threadId` and `runId` matter: the client correlates errors by run, and a
`RUN_ERROR` without a `runId` is treated as a session-level failure that clears
*every* active run. Provider messages are never forwarded verbatim — see
[Error handling](/guides/error-handling).

## Run state

`StepEventStateSnapshot` publishes a full state document; `StepEventStateDelta`
publishes an [RFC-6902](https://datatracker.ietf.org/doc/html/rfc6902) patch
against the last one:

```go
aikit.StepEvent{Type: aikit.StepEventStateSnapshot, State: json.RawMessage(`{"stage":"drafting"}`)}
aikit.StepEvent{Type: aikit.StepEventStateDelta,
    StatePatch: json.RawMessage(`[{"op":"replace","path":"/stage","value":"review"}]`)}
```

`ai-go` never diffs two states to derive a patch and never applies one — the
producer says what changed, and the consuming application applies it. A patch
whose shape is not a valid operation array fails the run with `RUN_ERROR` rather
than being dropped, because a consumer cannot otherwise tell a rejected update
from one that never arrived.

::: warning TanStack does not render state natively
Its stream processor has **no handler** for `STATE_SNAPSHOT` or `STATE_DELTA`,
so neither becomes a message part. They reach an application only through
`useChat`'s `onChunk` callback, which runs on every chunk *before* the processor:

```tsx
useChat({
  connection,
  onChunk: (chunk) => {
    if (chunk.type === 'STATE_SNAPSHOT') setAgentState(chunk.snapshot)
    if (chunk.type === 'STATE_DELTA') setAgentState((prev) => applyPatch(prev, chunk.delta))
  },
})
```
:::

State is echoed to the browser verbatim: it must never carry secrets, API keys,
or raw provider responses, and an unbounded state document becomes an unbounded
SSE frame.

A run that emits neither event writes no state frames. The separate
`STATE_SNAPSHOT` echo at an interrupt boundary is unaffected.

## Progressive structured output

`structured-output.complete` alone delivers the object only at the end. TanStack
routes assistant text into a structured-output part — and exposes a
progressively parsed `partial` — only for a message it saw *announced*:

```go
agui.Protocol(agui.WithStructuredOutputStart())
```

That emits `CUSTOM` named `structured-output.start` immediately after
`RUN_STARTED`. Without it the JSON lands in a plain text part and `partial`
never populates. It is opt-in because the engine reports structured output only
at the end of a run, long after the stream opened, so the encoder cannot infer
it.

## Tool approval through interrupt and resume

AG-UI has no in-band approval event. A run that needs a human decision suspends
and reports the pending interrupt on its terminal event:

```json
{
  "type": "RUN_FINISHED",
  "threadId": "t1",
  "runId": "r1",
  "outcome": {
    "type": "interrupt",
    "interrupts": [{
      "id": "approval_1",
      "reason": "tool_call",
      "message": "Approval required to run send_email",
      "toolCallId": "call_1",
      "responseSchema": { "type": "object", "properties": { "approved": { "type": "boolean" } } },
      "metadata": {
        "kind": "approval",
        "toolName": "send_email",
        "input": { "to": "a@b.test" },
        "tanstack:interruptBinding": { "v": 1, "kind": "tool-approval", "...": "..." }
      }
    }]
  }
}
```

`MESSAGES_SNAPSHOT` is sent immediately before it, carrying the request's own
history followed by the assistant turn. The client *replaces* its message list
with a snapshot rather than merging it, so publishing only the assistant turn
would delete the user's message and every earlier turn from the transcript — and
leave the resumed request with no user text. Prior messages go back out as the
exact bytes that arrived, so fields this package does not model survive.

The client resolves the interrupt and posts a **new** `RunAgentInput` carrying
`resume`:

```json
{"resume":[{"interruptId":"approval_1","status":"resolved","payload":{"approved":true}}]}
```

The engine's approval HMAC travels inside `tanstack:interruptBinding.signature`,
so a stateless server can authenticate the resume without retaining run state.
Read `Request.Extra["resume"]` to apply the decision — the approved/denied bit
travels **only** there, never in `messages`.

## Client-executed tools

`tool.NewClient` declares a tool this process never runs. The model sees it like
any other tool; when it is called, the runtime streams `TOOL_CALL_START` /
`TOOL_CALL_ARGS` / `TOOL_CALL_END` and then suspends the turn:

```go
render, err := tool.NewClient("render_chart", "Draw a chart in the page", schema)
```

The run ends with `RUN_FINISHED` — never `RUN_ERROR` — carrying a client-tool
interrupt:

```json
{
  "id": "client_tool_call_1",
  "reason": "tanstack:client_tool_execution",
  "toolCallId": "call_1",
  "metadata": { "kind": "client_tool", "toolName": "render_chart", "input": {"series": [1, 2]} }
}
```

The browser executes the tool and its output returns in the **next** request as
an ordinary `{"role":"tool"}` message, which the decoder already resolves
against the preceding assistant `toolCalls`. The client resubmits by itself; the
app never calls `sendMessage` again.

No `tanstack:interruptBinding` is attached. TanStack honors a binding only when
it carries schema digests computed by its own canonicalizer, and a binding that
fails that check routes to the same metadata-marked path as no binding at all —
so emitting one would add a dependency without adding a guarantee. The spellings
above are load-bearing: `metadata.kind` uses an underscore, and the `input` key
must be present (JSON `null` when arguments do not parse) because the client
tests for key presence.

::: warning A client tool's output is untrusted
The executor is the browser, so there is nothing for the server to
authenticate — unlike an approval resume, no HMAC is possible. Treat the
returned output as user-controlled input, and never give a client tool a name or
description implying server-side authority.
:::

The AI SDK v7 adapter needs no equivalent: there, a tool call with no result
followed by a clean finish already *is* the client-tool contract, which
`ainode` produces from the same events.

## What is not implemented

- Stream durability, resume-from-offset (`Last-Event-ID`, `?offset`), and thread
  hydration endpoints.
- `TEXT_MESSAGE_CHUNK`, `TOOL_CALL_CHUNK`, `ACTIVITY_*`, and `RAW`, which are
  outside TanStack's union.

Behavioral conformance against a live client is covered by the browser suite in
`conformance/`; the Go tests pin JSON shape, ordering, pairing, and terminal
behavior.

## Runnable playground

`test-ai-go/examples/24-tanstack-ag-ui/` pairs this endpoint with a Vite React
client under `web/`. It works without an API key, falling back to a demo model
that emits reasoning, text, and usage.

## See also

- [AI SDK v7 UI streams](/integrations/ui-streams) for the other adapter
- [Protocol extensions](/integrations/protocol-extensions) for the driver seam
- [Agent Runner](/core/agent-runner) for Runner ownership
