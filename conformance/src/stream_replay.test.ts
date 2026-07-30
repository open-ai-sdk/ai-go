import { readUIMessageStream, type UIMessage } from 'ai';
import { describe, expect, it } from 'vitest';

import { chunkStream, loadFixtures } from './fixtures.js';

type PartSummary = {
  type: string;
  state?: string;
  text?: string;
};

const expectedParts: Record<string, PartSummary[]> = {
  'deep-thinking-full': [
    { type: 'data-plan' },
    { type: 'data-steps' },
    { type: 'step-start' },
    {
      type: 'reasoning',
      state: 'done',
      text: 'I need to find the latest AI news. Let me use the web search tool.',
    },
    { type: 'tool-web_search', state: 'output-available' },
    { type: 'step-start' },
    {
      type: 'text',
      state: 'done',
      text:
        'The AI landscape in 2025 has seen remarkable progress. ' +
        'OpenAI released GPT-5, Google launched Gemini 2.0 Ultra, ' +
        'and Anthropic made Claude 4 available to users.',
    },
    { type: 'source-url' },
    { type: 'source-url' },
    { type: 'data-suggested-questions' },
    { type: 'data-usage' },
  ],
  'error-mid-stream': [
    { type: 'step-start' },
    {
      type: 'text',
      state: 'done',
      text: 'Let me help you with that. First, I need to look up some inform',
    },
  ],
  'reasoning-with-sources': [
    { type: 'step-start' },
    {
      type: 'reasoning',
      state: 'done',
      text: 'Let me think about this carefully. The user is asking about AI trends in 2025.',
    },
    {
      type: 'text',
      state: 'done',
      text: 'In 2025, the key AI trends include large multimodal models and agentic workflows.',
    },
    { type: 'source-url' },
    { type: 'source-url' },
  ],
  'text-only': [
    { type: 'step-start' },
    { type: 'text', state: 'done', text: 'Hello! How can I help you today?' },
  ],
  'tool-call-lifecycle': [
    { type: 'step-start' },
    { type: 'tool-search_documents', state: 'output-available' },
    { type: 'data-document-references' },
    { type: 'step-start' },
    {
      type: 'text',
      state: 'done',
      text: 'Based on the documents found, Go concurrency uses goroutines and channels.',
    },
    { type: 'data-usage' },
  ],
};

function summarize(message: UIMessage): PartSummary[] {
  return message.parts.map(part => {
    const value = part as { type: string; state?: string; text?: string };
    return {
      type: value.type,
      ...(value.state === undefined ? {} : { state: value.state }),
      ...(value.text === undefined ? {} : { text: value.text }),
    };
  });
}

describe('readUIMessageStream fixture replay', async () => {
  for (const fixture of await loadFixtures()) {
    it(`reconstructs ${fixture.name}`, async () => {
      let finalMessage: UIMessage | undefined;
      const errors: unknown[] = [];

      for await (const message of readUIMessageStream({
        stream: chunkStream(fixture.chunks),
        onError: error => errors.push(error),
      })) {
        finalMessage = message;
      }

      expect(finalMessage).toBeDefined();
      expect(summarize(finalMessage!)).toEqual(expectedParts[fixture.name]);
      if (fixture.name === 'error-mid-stream') {
        expect(errors).toHaveLength(1);
      } else {
        expect(errors).toHaveLength(0);
      }
    });
  }
});
