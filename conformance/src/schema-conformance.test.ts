import { readdirSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { collectValidatedChunks } from './client-pipeline.js';
import { parseSSEChunks, readFixture } from './parse-sse.js';

// Layer 1 — the schema layer.
//
// Every chunk ai-go emits must pass the client's own uiMessageChunkSchema, because
// DefaultChatTransport does `if (!chunk.success) throw chunk.error` on any failure
// (default-chat-transport.ts:28-30). A chunk that fails here does not render wrong; it
// takes down the whole stream in the browser.

const positiveFixtures = readdirSync(new URL('../fixtures/', import.meta.url).pathname)
  .filter(f => f.endsWith('.sse'))
  .sort();

describe('layer 1: framing', () => {
  it('found fixtures to check', () => {
    expect(positiveFixtures.length).toBeGreaterThan(0);
  });

  for (const name of positiveFixtures) {
    it(`${name} is well-framed and terminated`, () => {
      const raw = readFixture(name);
      // Throws on a missing [DONE], a frame without the data: prefix, or content
      // after the terminator — none of which the client itself would reject.
      const chunks = parseSSEChunks(raw);
      expect(chunks.length).toBeGreaterThan(0);
      for (const c of chunks) {
        expect(c).toHaveProperty('type');
        expect(typeof (c as { type: unknown }).type).toBe('string');
      }
    });
  }
});

describe('layer 1: uiMessageChunkSchema', () => {
  for (const name of positiveFixtures) {
    it(`${name} passes the real chunk schema`, async () => {
      const raw = readFixture(name);
      const chunks = await collectValidatedChunks(raw);

      // Every frame in the fixture must survive validation — a silently dropped
      // chunk would mean the stream validated but lost content.
      const rawCount = parseSSEChunks(raw).length;
      expect(chunks.length).toBe(rawCount);
    });
  }
});

describe('layer 1: every emitted type is a real protocol member', () => {
  // uiMessageChunkSchema is a union of literals plus the open data-${string} family.
  // Validation already enforces this, but naming the observed set makes an accidental
  // widening visible in the diff rather than only in a failure.
  const known = new Set([
    'start', 'start-step', 'finish-step', 'finish', 'abort', 'message-metadata', 'error',
    'text-start', 'text-delta', 'text-end',
    'reasoning-start', 'reasoning-delta', 'reasoning-end',
    'tool-input-start', 'tool-input-delta', 'tool-input-available', 'tool-input-error',
    'tool-output-available', 'tool-output-error', 'tool-output-denied',
    'tool-approval-request', 'tool-approval-response',
    'source-url', 'source-document', 'file', 'reasoning-file', 'custom',
  ]);

  for (const name of positiveFixtures) {
    it(`${name} emits only known chunk types`, () => {
      for (const c of parseSSEChunks(readFixture(name)) as { type: string }[]) {
        if (c.type.startsWith('data-')) continue;
        expect(known, `unexpected chunk type ${c.type} in ${name}`).toContain(c.type);
      }
    });
  }
});
