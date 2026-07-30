import { describe, expect, it } from 'vitest';
import { collectValidatedChunks, runClientProcessor } from './client-pipeline.js';
import { parseSSEChunks, parseSSEFrames, readFixture } from './parse-sse.js';

// The negative suite proves the harness can actually fail.
//
// Two rules it enforces on itself:
//
//  1. Each fixture must produce its SPECIFIC error, matched on the message the client
//     actually emits. "Something threw" would pass for a typo in the fixture path.
//  2. `onError` must have fired. Without that check, `terminateOnError: false` — the
//     library default — routes every error to onError and closes the stream normally,
//     so every negative fixture yields a plausible truncated message and the suite goes
//     green. That failure mode is the reason this file exists.

interface LayerTwoCase {
  fixture: string;
  /** Substring of the exact message from ai/src/ui/process-ui-message-stream.ts. */
  message: string;
  /** Line in that file where the throw lives, for traceability. */
  site: number;
}

const layerTwo: LayerTwoCase[] = [
  {
    fixture: 'invalid/text-delta-without-start.sse',
    message: 'Received text-delta for missing text part with ID "t0"',
    site: 439,
  },
  {
    fixture: 'invalid/reasoning-delta-without-start.sse',
    message: 'Received reasoning-delta for missing reasoning part with ID "r0"',
    site: 500,
  },
  {
    fixture: 'invalid/tool-input-delta-without-start.sse',
    message: 'Received tool-input-delta for missing tool call with ID "call_1"',
    site: 622,
  },
  {
    fixture: 'invalid/unknown-approval-id.sse',
    message: 'No tool invocation found for approval ID "appr_does_not_exist"',
    site: 163,
  },
];

describe('layer 2 negatives: the stream processor must throw', () => {
  for (const c of layerTwo) {
    it(`${c.fixture} throws at process-ui-message-stream.ts:${c.site}`, async () => {
      const { errors, thrown } = await runClientProcessor(readFixture(c.fixture));

      // Rule 2: onError fired. This is what makes the assertion meaningful.
      expect(errors.length, 'onError did not fire').toBeGreaterThan(0);
      // Rule 1: the specific error, not merely an error.
      expect(String(errors[0])).toContain(c.message);
      // terminateOnError must surface it to the consumer as well.
      expect(thrown, 'terminateOnError did not reject the stream').toBeDefined();
      expect(String(thrown)).toContain(c.message);
    });
  }

  it('these fixtures pass layer 1, so only layer 2 can catch them', async () => {
    // If a negative fixture failed the schema instead, it would be testing layer 1
    // while claiming to test the processor.
    for (const c of layerTwo) {
      await expect(
        collectValidatedChunks(readFixture(c.fixture)),
        `${c.fixture} should be schema-valid`,
      ).resolves.toBeDefined();
    }
  });

  it('terminateOnError:false would hide every one of them', async () => {
    // The meta-test the phase plan calls for. With the library default, each malformed
    // fixture yields a message and no exception — so a suite written without
    // terminateOnError would be green and worthless.
    for (const c of layerTwo) {
      const { message, thrown, errors } = await runClientProcessor(readFixture(c.fixture), {
        terminateOnError: false,
      });
      expect(thrown, `${c.fixture} threw even with terminateOnError:false`).toBeUndefined();
      expect(message, `${c.fixture} produced no message`).toBeDefined();
      // The error still reaches onError — which is the only reason the default is
      // survivable at all, and why asserting onError fired is the load-bearing check.
      expect(errors.length).toBeGreaterThan(0);
    }
  });
});

describe('layer 1 negatives: the chunk schema must reject', () => {
  const badSchema = 'invalid-schema/unknown-chunk-type.sse';

  it(`${badSchema} fails validation`, async () => {
    await expect(collectValidatedChunks(readFixture(badSchema))).rejects.toThrow();
  });

  it('the rejection names the offending value', async () => {
    let err: unknown;
    try {
      await collectValidatedChunks(readFixture(badSchema));
    } catch (e) {
      err = e;
    }
    expect(err).toBeDefined();
    // The schema is a union of type literals, so the failure has to mention the value
    // that matched no member.
    expect(String(err)).toContain('text-suffix');
  });

  it('an unknown chunk type is invisible to layer 2 alone', async () => {
    // The processor's default: branch ignores unknown types
    // (process-ui-message-stream.ts:922-923). Without schema validation in front of it
    // this fixture renders as if nothing were wrong — which is why layer 1 is a
    // separate layer and not an implementation detail of layer 2.
    const { readUIMessageStream } = await import('ai');

    const chunks = parseSSEChunks(readFixture(badSchema));
    const stream = new ReadableStream<any>({
      start(c) {
        for (const ch of chunks) c.enqueue(ch);
        c.close();
      },
    });

    const errors: unknown[] = [];
    let last: unknown;
    for await (const m of readUIMessageStream({
      stream,
      terminateOnError: true,
      onError: e => errors.push(e),
    })) {
      last = m;
    }

    expect(errors, 'the processor unexpectedly rejected an unknown type').toEqual([]);
    expect(last, 'the processor produced nothing').toBeDefined();
  });
});

describe('framing negatives: caught before either client layer', () => {
  it('a stream without [DONE] is rejected', () => {
    expect(() => parseSSEFrames('data: {"type":"start-step"}\n\n')).toThrow(/terminator/);
  });

  it('a frame without the data: prefix is rejected', () => {
    expect(() => parseSSEFrames('event: message\n\ndata: [DONE]\n\n')).toThrow(/data: /);
  });

  it('content after [DONE] is rejected', () => {
    expect(() => parseSSEFrames('data: [DONE]\n\ndata: {"type":"start-step"}\n\n')).toThrow(
      /after \[DONE\]/,
    );
  });

  it('a stream not ending in a frame terminator is rejected', () => {
    expect(() => parseSSEFrames('data: [DONE]')).toThrow(/frame terminator/);
  });
});
