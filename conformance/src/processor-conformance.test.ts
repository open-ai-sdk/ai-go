import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { normalize, runClientProcessor } from './client-pipeline.js';
import { FIXTURE_ROOT, readFixture } from './parse-sse.js';

// Layer 2 — the behavioural layer.
//
// Passing the schema only proves each chunk is individually well-formed. This runs the
// whole stream through the client's actual state machine (readUIMessageStream, which
// wraps processUIMessageStream) and asserts the rendered message parts. That is where
// ordering and state-machine divergence shows up.
//
// The .expected.json files are hand-authored from reading
// ai/src/ui/process-ui-message-stream.ts. If they were generated from Go output the
// test would assert only that Go agrees with itself.

const fixtures = readdirSync(FIXTURE_ROOT)
  .filter(f => f.endsWith('.expected.json'))
  .map(f => f.replace(/\.expected\.json$/, ''))
  .sort();

function expectedParts(name: string): unknown[] {
  return JSON.parse(readFileSync(join(FIXTURE_ROOT, `${name}.expected.json`), 'utf8'));
}

describe('layer 2: rendered message parts', () => {
  it('every .sse fixture has hand-written expectations', () => {
    const sse = readdirSync(FIXTURE_ROOT).filter(f => f.endsWith('.sse')).sort();
    const missing = sse
      .map(f => f.replace(/\.sse$/, ''))
      .filter(n => !fixtures.includes(n));
    expect(missing, 'fixtures without .expected.json').toEqual([]);
  });

  for (const name of fixtures) {
    it(`${name} renders the expected parts`, async () => {
      const { message, errors, thrown } = await runClientProcessor(readFixture(`${name}.sse`));

      // The `error` fixture is the one case that legitimately surfaces an error: the
      // client calls onError and breaks, and terminateOnError then rejects the stream.
      // Every other fixture must be completely clean.
      if (name !== 'error') {
        expect(errors, `${name} routed errors to onError`).toEqual([]);
        expect(thrown, `${name} threw`).toBeUndefined();
      }

      expect(message, `${name} produced no message`).toBeDefined();
      expect(normalize(message!.parts)).toEqual(expectedParts(name));
    });
  }
});

describe('layer 2: step boundaries', () => {
  // A mis-derived start-step is the sharpest silent failure in the protocol. The client
  // never throws for it: getToolInvocation reverse-scans the whole message
  // (process-ui-message-stream.ts:131-142) and updateToolPart pushes a duplicate part
  // (:255-280). So the ONLY way layer 2 detects it is by asserting where each
  // step-start part sits — which is why the expectations pin positions, not counts.
  it('text-multi-segment-across-tool has step-start at exactly indices 0 and 3', async () => {
    const { message } = await runClientProcessor(
      readFixture('text-multi-segment-across-tool.sse'),
    );
    const parts = normalize(message!.parts) as { type: string }[];
    const stepIndices = parts
      .map((p, i) => (p.type === 'step-start' ? i : -1))
      .filter(i => i >= 0);

    expect(stepIndices).toEqual([0, 3]);
    // The second text segment must be a separate part, not an append to the first.
    expect(parts[1]).toMatchObject({ type: 'text', text: 'Let me check.' });
    expect(parts[4]).toMatchObject({ type: 'text', text: 'It is 31C.' });
  });

  it('every expected file with two steps pins both positions', () => {
    for (const name of fixtures) {
      const parts = expectedParts(name) as { type: string }[];
      const steps = parts.filter(p => p.type === 'step-start').length;
      if (steps > 1) {
        // Position is implied by array index in the expectation, so this asserts the
        // expectation itself is precise enough to detect a shifted boundary.
        const idx = parts.map((p, i) => (p.type === 'step-start' ? i : -1)).filter(i => i >= 0);
        expect(idx.length, `${name}`).toBe(steps);
        expect(new Set(idx).size, `${name} has duplicate step indices`).toBe(steps);
      }
    }
  });
});

describe('layer 2: protocol behaviours worth pinning', () => {
  it('a reasoning signature survives on providerMetadata, not as an empty delta', async () => {
    const raw = readFixture('reasoning-with-signature.sse');
    expect(raw).not.toContain('"delta":""');

    const { message } = await runClientProcessor(raw);
    const reasoning = (normalize(message!.parts) as any[]).find(p => p.type === 'reasoning');
    expect(reasoning.providerMetadata.anthropic.signature).toBe('sig-abc123');
    expect(reasoning.text).toBe('Thinking about it. Done thinking.');
  });

  it('an error chunk does not unwind already-rendered parts', async () => {
    const { message, errors } = await runClientProcessor(readFixture('error.sse'));
    expect(errors.length).toBe(1);

    const parts = normalize(message!.parts) as any[];
    const text = parts.find(p => p.type === 'text');
    // The partial text stays, and stays open — the client never closes it.
    expect(text.text).toBe('Partial answer');
    expect(text.state).toBe('streaming');
  });

  it('a redacted error reaches the client with no provider detail', async () => {
    const { errors } = await runClientProcessor(readFixture('error.sse'));
    const text = String(errors[0]);
    expect(text).toContain('500');
    for (const secret of ['acme', 'req_1', 'secret']) {
      expect(text, `leaked ${secret}`).not.toContain(secret);
    }
  });

  it('a denied approval renders as output-denied, never as an error', async () => {
    const { message, errors } = await runClientProcessor(readFixture('tool-approval-denied.sse'));
    expect(errors).toEqual([]);

    const tool = (normalize(message!.parts) as any[]).find(p => p.type === 'tool-sendEmail');
    expect(tool.state).toBe('output-denied');
    expect(tool.approval.approved).toBe(false);
    expect(tool).not.toHaveProperty('output');
  });

  it('providerExecuted round-trips as an explicit false', async () => {
    const { message } = await runClientProcessor(readFixture('tool-server-executed.sse'));
    const tool = (normalize(message!.parts) as any[]).find(p => p.type === 'tool-getWeather');
    // Present-and-false, not absent: this is what the map-based chunk encoding buys.
    expect(tool).toHaveProperty('providerExecuted');
    expect(tool.providerExecuted).toBe(false);
  });

  it('tool-input-error moves the unparseable input to rawInput', async () => {
    // process-ui-message-stream.ts:726-729 sets input:undefined, rawInput:chunk.input.
    // So a producer must send the raw partial JSON as `input` and expect it back under
    // a different key.
    const { message } = await runClientProcessor(readFixture('tool-input-error.sse'));
    const tool = (normalize(message!.parts) as any[]).find(p => p.type === 'tool-getWeather');
    expect(tool.state).toBe('output-error');
    expect(tool.rawInput).toBe('{"city":');
    expect(tool).not.toHaveProperty('input');
  });
});
