// Package agui implements the AG-UI event stream consumed by TanStack AI
// clients, based on the @ag-ui/core event and RunAgentInput contracts.
//
// It covers text, reasoning, tool lifecycle, tool approval via interrupt and
// resume, client-executed tools, sources, files, structured output, usage, run
// state, and message snapshots.
//
// STATE_SNAPSHOT and STATE_DELTA are produced from the matching engine events,
// but TanStack's stream processor has no handler for either, so neither becomes
// a message part. They reach an application only through useChat's onChunk
// callback, which runs before the processor. Nothing in this package applies a
// patch; the consuming application owns that.
//
// Several omissions follow from the client contract rather than from scope. The
// deprecated THINKING_* family is absent from TanStack AI's StreamChunk union
// and would be ignored, so REASONING_* carries thinking instead.
// TEXT_MESSAGE_CHUNK, TOOL_CALL_CHUNK, ACTIVITY_*, and RAW are outside that
// union as well. There is no [DONE] sentinel: RUN_FINISHED terminates an AG-UI
// stream, and TanStack's SSE parser reports [DONE] as deprecated. STEP_STARTED
// and STEP_FINISHED are opt-in through WithStepEvents, because TanStack reads
// them as reasoning and renders a contentless one as an empty thinking part.
//
// Stream durability, resume-from-offset, and thread hydration endpoints are not
// implemented.
package agui
