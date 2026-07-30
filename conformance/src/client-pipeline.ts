import { parseJsonEventStream } from '@ai-sdk/provider-utils';
import { readUIMessageStream, uiMessageChunkSchema, type UIMessageChunk } from 'ai';
import type { UIMessage } from 'ai';
import { fixtureByteStream } from './parse-sse.js';

/**
 * The client's real validation path.
 *
 * This is a copy of DefaultChatTransport.processResponseStream
 * (ai/src/ui/default-chat-transport.ts:19-34) — the same parseJsonEventStream call
 * against the same uiMessageChunkSchema, and the same `if (!chunk.success) throw`.
 * Re-implementing validation instead would only assert our reading of the schema.
 */
export function validateChunks(raw: string): ReadableStream<UIMessageChunk> {
  return parseJsonEventStream({
    stream: fixtureByteStream(raw),
    schema: uiMessageChunkSchema,
  }).pipeThrough(
    new TransformStream<{ success: boolean; value?: UIMessageChunk; error?: unknown }, UIMessageChunk>({
      transform(chunk, controller) {
        if (!chunk.success) throw chunk.error;
        controller.enqueue(chunk.value!);
      },
    }),
  );
}

/** Collects every chunk a fixture yields, throwing on the first schema failure. */
export async function collectValidatedChunks(raw: string): Promise<UIMessageChunk[]> {
  const out: UIMessageChunk[] = [];
  const reader = validateChunks(raw).getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    out.push(value);
  }
  return out;
}

export interface RunResult {
  /** The last message state the stream emitted, or undefined if it never emitted one. */
  message: UIMessage | undefined;
  /** Every error routed to onError, in order. */
  errors: unknown[];
  /** Set when iterating the stream itself threw — which only happens with terminateOnError. */
  thrown: unknown;
}

/**
 * Runs a fixture through the client's stream processor.
 *
 * `terminateOnError: true` is not optional. The default is false
 * (read-ui-message-stream.ts:29), which routes every error to onError and then closes
 * the stream *normally* — so a malformed stream yields a plausible truncated message
 * and no exception. Under that default every negative fixture would "pass".
 */
export async function runClientProcessor(
  raw: string,
  opts: { terminateOnError?: boolean } = {},
): Promise<RunResult> {
  const { terminateOnError = true } = opts;
  const errors: unknown[] = [];
  let message: UIMessage | undefined;
  let thrown: unknown;

  try {
    const stream = readUIMessageStream({
      stream: validateChunks(raw),
      terminateOnError,
      onError: e => errors.push(e),
    });
    for await (const m of stream) {
      message = m;
    }
  } catch (e) {
    thrown = e;
  }

  return { message, errors, thrown };
}

/**
 * Strips keys whose value is undefined, recursively.
 *
 * The processor assigns optional fields unconditionally — `providerMetadata:
 * chunk.providerMetadata` leaves the key present with an undefined value
 * (process-ui-message-stream.ts:426). structuredClone keeps those keys, so a raw
 * comparison against hand-written JSON would fail on fields that are semantically
 * absent. A JSON round-trip is the same normalization the network performs.
 */
export function normalize<T>(value: T): T {
  return JSON.parse(JSON.stringify(value ?? null));
}
