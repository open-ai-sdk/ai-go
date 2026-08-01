import { validateTypes } from '@ai-sdk/provider-utils';
import { readFileSync } from 'node:fs';

import { installedUIMessageChunkSchema } from './union.js';

const chunks = JSON.parse(readFileSync(0, 'utf8')) as unknown[];
for (const chunk of chunks) {
  await validateTypes({ schema: installedUIMessageChunkSchema, value: chunk });
}
