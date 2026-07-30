import { readFile, writeFile } from 'node:fs/promises';

import {
  dataChunkPrefix,
  installedUIMessageChunkTypes,
} from './union.js';

const outputURL = new URL('./ui_message_chunk_types.json', import.meta.url);
const generated = `${JSON.stringify(
  {
    source: 'ai@7.0.35/src/ui-message-stream/ui-message-chunks.ts',
    literalTypes: await installedUIMessageChunkTypes(),
    prefixTypes: [dataChunkPrefix],
  },
  null,
  2,
)}\n`;

if (process.argv.includes('--check')) {
  const current = await readFile(outputURL, 'utf8').catch(() => '');
  if (current !== generated) {
    console.error(
      'ui_message_chunk_types.json is stale; run npm run union:generate',
    );
    process.exitCode = 1;
  }
} else {
  await writeFile(outputURL, generated);
}
