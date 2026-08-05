import { createHash, createHmac } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vite-plus/test';

type Vector = {
  name: string;
  secret: string;
  approvalId: string;
  toolCallId: string;
  toolName: string;
  input: unknown;
  digest: string;
  signature: string;
};

function canonicalJSON(value: unknown): string {
  if (Array.isArray(value)) {
    return `[${value.map(canonicalJSON).join(',')}]`;
  }
  if (value !== null && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>).sort(([a], [b]) =>
      a < b ? -1 : a > b ? 1 : 0,
    );
    return `{${entries.map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`).join(',')}}`;
  }
  return JSON.stringify(value);
}

function base64url(bytes: Buffer): string {
  return bytes.toString('base64url');
}

const vectors = JSON.parse(
  readFileSync(
    new URL('../../uistream/ainode/testdata/tool_approval_vectors.json', import.meta.url),
    'utf8',
  ),
) as Vector[];

describe('tool approval signature vectors', () => {
  for (const vector of vectors) {
    it(vector.name, () => {
      const digest = base64url(createHash('sha256').update(canonicalJSON(vector.input)).digest());
      const payload = JSON.stringify([
        'ai-sdk-tool-approval-v1',
        vector.approvalId,
        vector.toolCallId,
        vector.toolName,
        digest,
      ]);
      const signature = base64url(createHmac('sha256', vector.secret).update(payload).digest());
      expect(digest).toBe(vector.digest);
      expect(signature).toBe(vector.signature);
    });
  }
});
