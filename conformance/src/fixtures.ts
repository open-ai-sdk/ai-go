import { readFile, readdir } from 'node:fs/promises';
import { basename, join } from 'node:path';

import type { UIMessageChunk } from 'ai';

export const fixturesDir = new URL('../fixtures/', import.meta.url);

export type Fixture = {
  name: string;
  chunks: unknown[];
  raw: string;
};

export async function loadFixtures(): Promise<Fixture[]> {
  const names = (await readdir(fixturesDir))
    .filter(name => name.endsWith('.jsonl'))
    .sort();

  return Promise.all(
    names.map(async name => {
      const path = join(fixturesDir.pathname, name);
      const raw = await readFile(path, 'utf8');
      return {
        name: basename(name, '.jsonl'),
        chunks: raw
          .split(/\r?\n/)
          .filter(line => line.startsWith('data: '))
          .map(line => line.slice('data: '.length))
          .filter(payload => payload !== '[DONE]')
          .map(payload => JSON.parse(payload)),
        raw,
      };
    }),
  );
}

export function chunkStream(chunks: unknown[]): ReadableStream<UIMessageChunk> {
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(chunk as UIMessageChunk);
      }
      controller.close();
    },
  });
}
