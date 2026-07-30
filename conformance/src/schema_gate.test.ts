import { validateTypes } from '@ai-sdk/provider-utils';
import { describe, expect, it } from 'vitest';

import { loadFixtures } from './fixtures.js';
import { installedUIMessageChunkSchema } from './union.js';

describe('ai@7.0.35 UI message chunk schema', async () => {
  for (const fixture of await loadFixtures()) {
    it(`accepts every ${fixture.name} frame`, async () => {
      for (const [index, chunk] of fixture.chunks.entries()) {
        await expect(
          validateTypes({
            schema: installedUIMessageChunkSchema,
            value: chunk,
          }),
          `frame ${index + 1}: ${JSON.stringify(chunk)}`,
        ).resolves.toEqual(chunk);
      }
    });
  }
});
