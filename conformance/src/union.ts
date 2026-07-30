import { readFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';

import { uiMessageChunkSchema } from 'ai';

const require = createRequire(import.meta.url);
const aiPackageDir = dirname(require.resolve('ai/package.json'));
const chunkSourcePath = join(aiPackageDir, 'src', 'ui-message-stream', 'ui-message-chunks.ts');

export const dataChunkPrefix = 'data-' as const;

export async function installedUIMessageChunkTypes(): Promise<string[]> {
  const source = await readFile(chunkSourcePath, 'utf8');
  const literalTypes = [...source.matchAll(/\btype:\s*z\.literal\('([^']+)'\)/g)].map(
    (match) => match[1],
  );

  if (
    !source.includes(`value.startsWith('${dataChunkPrefix}')`) ||
    !source.includes('z.custom<`data-${string}`>')
  ) {
    throw new Error(`ai@7.0.35 no longer exposes the expected ${dataChunkPrefix} prefix member`);
  }

  const uniqueTypes = [...new Set(literalTypes)].sort();
  if (uniqueTypes.length !== 27) {
    throw new Error(`expected 27 literal UI message chunk types, found ${uniqueTypes.length}`);
  }
  return uniqueTypes;
}

// Importing the schema here is intentional: tests and generation share the
// exact installed contract instead of a handwritten TypeScript union.
export const installedUIMessageChunkSchema = uiMessageChunkSchema;
