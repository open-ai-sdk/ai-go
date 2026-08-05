import { EventSchemas } from '@ag-ui/core';
import { afterAll, beforeAll, describe, expect, it } from 'vite-plus/test';

import { eventTypes, parseAGUIStream, startGoServer, type GoServer } from './go-server.js';

let server: GoServer;

beforeAll(async () => {
  server = await startGoServer();
}, 120_000);

afterAll(async () => {
  await server?.stop();
});

async function runScenario(scenario: string) {
  const response = await fetch(`${server.url}/ag-ui?scenario=${scenario}`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      threadId: 'thread-1',
      runId: 'run-1',
      state: {},
      tools: [],
      context: [],
      forwardedProps: {},
      messages: [
        // TanStack's wire format keeps `parts` on the anchor message and adds a
        // `content` mirror.
        { id: 'user-1', role: 'user', parts: [{ type: 'text', text: 'hello' }], content: 'hello' },
      ],
    }),
  });
  const body = await response.text();
  return { response, body, ...parseAGUIStream(body) };
}

describe('live Go AG-UI stream', () => {
  it('serves an SSE content type', async () => {
    const { response } = await runScenario('text');
    expect(response.status).toBe(200);
    expect(response.headers.get('content-type')).toBe('text/event-stream');
  });

  it('emits only events @ag-ui/core accepts', async () => {
    for (const scenario of ['text', 'tool', 'approval', 'reasoning', 'rich', 'error']) {
      const { events } = await runScenario(scenario);
      expect(events.length, `${scenario} produced no events`).toBeGreaterThan(0);
      for (const event of events) {
        const result = EventSchemas.safeParse(event);
        expect(
          result.success,
          `${scenario}: ${JSON.stringify(event)} -> ${JSON.stringify(result.error?.issues)}`,
        ).toBe(true);
      }
    }
  });

  // RUN_FINISHED terminates an AG-UI stream. TanStack's SSE parser reports a
  // trailing [DONE] as a deprecated server.
  it('terminates without a [DONE] sentinel', async () => {
    for (const scenario of ['text', 'tool', 'rich']) {
      const { events, sawDone } = await runScenario(scenario);
      expect(sawDone, `${scenario} emitted [DONE]`).toBe(false);
      expect(eventTypes(events).at(-1)).toBe('RUN_FINISHED');
    }
  });

  // TanStack's processor turns a contentless STEP_FINISHED into a thinking
  // part, so bare step markers render an empty "thinking" block per step.
  it('omits STEP_* markers that would render as empty thinking parts', async () => {
    for (const scenario of ['text', 'tool', 'reasoning', 'rich']) {
      const { body } = await runScenario(scenario);
      expect(body, `${scenario} emitted STEP_STARTED`).not.toContain('STEP_STARTED');
      expect(body, `${scenario} emitted STEP_FINISHED`).not.toContain('STEP_FINISHED');
    }
  });

  it('orders the text lifecycle', async () => {
    const { events } = await runScenario('text');
    expect(eventTypes(events)).toEqual([
      'RUN_STARTED',
      'TEXT_MESSAGE_START',
      'TEXT_MESSAGE_CONTENT',
      'TEXT_MESSAGE_END',
      'RUN_FINISHED',
    ]);
  });

  // REASONING_MESSAGE_CONTENT is the event TanStack's processor consumes;
  // THINKING_* is deprecated and would render nothing.
  it('closes reasoning before assistant text and never emits THINKING_*', async () => {
    const { events, body } = await runScenario('reasoning');
    expect(eventTypes(events)).toEqual([
      'RUN_STARTED',
      'REASONING_START',
      'REASONING_MESSAGE_START',
      'REASONING_MESSAGE_CONTENT',
      'REASONING_MESSAGE_END',
      'REASONING_END',
      'TEXT_MESSAGE_START',
      'TEXT_MESSAGE_CONTENT',
      'TEXT_MESSAGE_END',
      'RUN_FINISHED',
    ]);
    expect(body).not.toContain('THINKING_');
  });

  it('gives every tool call a start before its arguments and a distinct result message', async () => {
    const { events } = await runScenario('tool');
    const types = eventTypes(events);
    expect(types).toContain('TOOL_CALL_START');
    expect(types.indexOf('TOOL_CALL_START')).toBeLessThan(types.indexOf('TOOL_CALL_END'));

    const assistantMessages = new Set(
      events.filter((e) => e.type === 'TEXT_MESSAGE_START').map((e) => e.messageId),
    );
    for (const result of events.filter((e) => e.type === 'TOOL_CALL_RESULT')) {
      // A tool result is its own AG-UI message; reusing the assistant message id
      // would give two messages one identity.
      expect(assistantMessages.has(result.messageId)).toBe(false);
      expect(result.role).toBe('tool');
    }
  });

  it('publishes sources, files, and structured output as CUSTOM events', async () => {
    const { events } = await runScenario('rich');
    const custom = events.filter((event) => event.type === 'CUSTOM');
    const names = custom.map((event) => String(event.name));
    expect(names.sort((left, right) => left.localeCompare(right))).toEqual([
      'file',
      'source',
      'structured-output.complete',
    ]);

    const file = custom.find((event) => event.name === 'file')!;
    expect((file.value as { url: string }).url).toMatch(/^data:image\/png;base64,/);
  });

  // AG-UI has no usage field; TanStack reads RUN_FINISHED.usage as TokenUsage.
  it('reports usage in the TanStack TokenUsage shape', async () => {
    const { events } = await runScenario('rich');
    const finished = events.at(-1)!;
    expect(finished.type).toBe('RUN_FINISHED');
    expect(finished.usage).toEqual({
      promptTokens: 11,
      completionTokens: 22,
      totalTokens: 33,
    });
    expect(finished.finishReason).toBe('stop');
  });

  it('suspends on tool approval with an interrupt outcome', async () => {
    const { events } = await runScenario('approval');
    const types = eventTypes(events);
    expect(types).toContain('MESSAGES_SNAPSHOT');
    expect(types.at(-1)).toBe('RUN_FINISHED');
    // The snapshot must precede the terminal event: the client rebuilds its
    // message list from it before rendering the prompt.
    expect(types.indexOf('MESSAGES_SNAPSHOT')).toBeLessThan(types.length - 1);

    const finished = events.at(-1) as {
      outcome?: { type: string; interrupts: Array<Record<string, unknown>> };
    };
    expect(finished.outcome?.type).toBe('interrupt');
    expect(finished.outcome?.interrupts).toHaveLength(1);

    const interrupt = finished.outcome!.interrupts[0];
    expect(interrupt.reason).toBe('tool_call');
    expect(interrupt.toolCallId).toBe('tool-call-1');

    const metadata = interrupt.metadata as Record<string, unknown>;
    expect(metadata.kind).toBe('approval');
    expect(metadata['tanstack:interruptBinding']).toMatchObject({
      v: 1,
      kind: 'tool-approval',
      toolCallId: 'tool-call-1',
    });
  });

  it('redacts a terminal error and scopes it to the run', async () => {
    const { events } = await runScenario('error');
    const runError = events.at(-1) as Record<string, unknown>;
    expect(runError.type).toBe('RUN_ERROR');
    expect(runError.message).toBe('stream error');
    // A RUN_ERROR without a runId is treated as a session-level failure and
    // clears every active run.
    expect(runError.runId).toBe('run-1');
    expect(runError.threadId).toBe('thread-1');
  });

  it('accepts the reasoning and activity rows TanStack fans out', async () => {
    const response = await fetch(`${server.url}/ag-ui?scenario=text`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        threadId: 'thread-1',
        runId: 'run-1',
        messages: [
          { id: 'u1', role: 'user', parts: [{ type: 'text', text: 'hi' }], content: 'hi' },
          { id: 'a1-reasoning-p1', role: 'reasoning', content: 'thinking' },
          { id: 'a1', role: 'assistant', content: 'hello' },
        ],
      }),
    });
    expect(response.status).toBe(200);
  });
});
