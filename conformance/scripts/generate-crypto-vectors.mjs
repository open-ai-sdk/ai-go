// Generates the shared approval-crypto vectors, in TypeScript, because TS is the
// reference implementation. Go is then made to match.
//
//   node scripts/generate-crypto-vectors.mjs
//
// Output lands in ../aisdk/testdata/ so the Go tests and the TS suite read the same
// bytes. Generating them from Go instead would make the whole exercise circular: the
// failure this guards against is Go being *more permissive* than the reference, and only
// TS-authored expectations can catch that.
import { writeFileSync } from 'node:fs';
import { createHash, createHmac } from 'node:crypto';

const OUT = new URL('../../aisdk/testdata/', import.meta.url).pathname;
const enc = new TextEncoder();

// --- the reference algorithm, transcribed from
//     ai/src/util/canonical-hash.ts and ai/src/generate-text/tool-approval-signature.ts

function canonicalJSON(value) {
  if (value === null || value === undefined) return JSON.stringify(value);
  if (typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`;
  const keys = Object.keys(value).sort();
  return `{${keys.map(k => `${JSON.stringify(k)}:${canonicalJSON(value[k])}`).join(',')}}`;
}

function toBase64url(buf) {
  return Buffer.from(buf).toString('base64')
    .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function hashCanonical(value) {
  return toBase64url(createHash('sha256').update(enc.encode(canonicalJSON(value))).digest());
}

const V1 = 'ai-sdk-tool-approval-v1';
const V2 = 'ai-go-tool-approval-v2';

function signV1(secret, approvalId, toolCallId, toolName, input) {
  const payload = JSON.stringify([V1, approvalId, toolCallId, toolName, hashCanonical(input)]);
  return toBase64url(createHmac('sha256', secret).update(enc.encode(payload)).digest());
}

function signV2(secret, approvalId, toolCallId, toolName, input, principalId, chatId, iat) {
  const payload = JSON.stringify([
    V2, approvalId, toolCallId, toolName, hashCanonical(input), principalId, chatId, iat,
  ]);
  return toBase64url(createHmac('sha256', secret).update(enc.encode(payload)).digest());
}

// --- canonical vectors ---
//
// Each entry names the divergence it exists to catch. Measured behaviours, not guesses:
// JS `.sort()` orders by UTF-16 code unit, so a non-BMP key (first code unit 0xD83D)
// sorts BEFORE U+E000..U+FFFF, while Go byte order puts them after. An emoji paired with
// a Latin-1 key agrees in both languages and therefore cannot detect the bug.
const canonicalCases = [
  { name: 'empty-object', input: {} },
  { name: 'empty-array', input: [] },
  { name: 'null', input: null },
  { name: 'scalar-string', input: 'hello' },
  { name: 'scalar-number', input: 42 },
  { name: 'scalar-bool', input: true },
  { name: 'ascii-keys-sorted', input: { b: 1, a: 2, C: 3, A: 4 } },
  {
    name: 'nonbmp-key-vs-private-use-key',
    note: 'the vector that actually detects the sort divergence: U+1F600 vs U+E000',
    input: { '\u{1F600}': 1, '': 2, a: 3 },
  },
  {
    name: 'nonbmp-key-vs-fullwidth-key',
    note: 'U+1F600 vs U+FF21 — same class of divergence, inside BMP',
    input: { '\u{1F600}': 1, 'Ａ': 2 },
  },
  {
    name: 'nonbmp-key-vs-latin1-key',
    note: 'agrees in both languages; included to prove the sort fix does not break it',
    input: { '\u{1F600}': 1, 'é': 2 },
  },
  {
    name: 'html-chars-in-value',
    note: 'JSON.stringify does not escape < > &; Go must use SetEscapeHTML(false)',
    input: { html: '<a href="x">&amp;</a>', cmp: 'a < b && c > d' },
  },
  { name: 'html-chars-in-key', input: { '<k&y>': 1 } },
  {
    name: 'lone-surrogate-key',
    note: 'JSON.stringify emits \\ud800 as-is; Go replaces invalid UTF-8 with U+FFFD',
    input: { '\uD800': 1, ok: 2 },
  },
  {
    name: 'lone-surrogate-value',
    input: { v: '\uD800' },
  },
  {
    name: 'floats',
    note: 'identical in both when Go uses encoding/json rather than strconv',
    input: [1e21, 1e-7, 1e-21, 1e20, 1.5e300, 5e-324, 0.1, 1.2345678901234567e19, -0.0],
  },
  { name: 'nested', input: { z: [1, { b: 2, a: [3, { d: 4, c: 5 }] }], y: null } },
  { name: 'unicode-values', input: { vi: 'Tiếng Việt', jp: '日本語', emoji: '🇻🇳👍🏽' } },
  { name: 'deep-64', input: buildDeep(64) },
  {
    name: 'array-with-null',
    note: 'null inside an array is "null", distinct from an absent element',
    input: [1, null, 'x'],
  },
];

function buildDeep(n) {
  let v = 1;
  for (let i = 0; i < n; i++) v = { a: v };
  return v;
}

const canonicalVectors = canonicalCases.map(c => ({
  name: c.name,
  ...(c.note ? { note: c.note } : {}),
  input: c.input,
  canonicalJSON: canonicalJSON(c.input),
  hashCanonical: hashCanonical(c.input),
}));

// --- approval vectors ---

const SECRET_A = 'secret-alpha-0123456789';
const SECRET_B = 'secret-bravo-9876543210';
const base = { approvalId: 'appr_1', toolCallId: 'call_1', toolName: 'deleteFile' };
const input = { path: '/tmp/x', force: false };
const goodV1 = signV1(SECRET_A, base.approvalId, base.toolCallId, base.toolName, input);

const P = { principalId: 'user_42', chatId: 'chat_7', iat: 1785000000 };
const goodV2 = signV2(SECRET_A, base.approvalId, base.toolCallId, base.toolName, input,
  P.principalId, P.chatId, P.iat);

/** Re-encodes a base64url signature as padded standard base64. */
function toStandardPadded(b64url) {
  const std = b64url.replace(/-/g, '+').replace(/_/g, '/');
  const pad = (4 - (std.length % 4)) % 4;
  return std + '='.repeat(pad);
}

const approvalVectors = [
  { name: 'v1-valid', version: 'v1', secret: SECRET_A, ...base, input,
    signature: goodV1, expect: 'accept' },

  { name: 'v1-valid-padded-signature', version: 'v1', secret: SECRET_A, ...base, input,
    signature: toStandardPadded(goodV1).replace(/\+/g, '-').replace(/\//g, '_'),
    expect: 'accept',
    note: 'trailing = padding; TS atob accepts it, Go RawURLEncoding does not' },

  { name: 'v1-valid-standard-base64-signature', version: 'v1', secret: SECRET_A, ...base,
    input, signature: toStandardPadded(goodV1), expect: 'accept',
    note: '+ and / instead of - and _; TS maps them back before atob' },

  { name: 'v1-tampered-input-same-signature', version: 'v1', secret: SECRET_A, ...base,
    input: { path: '/etc/passwd', force: false }, signature: goodV1, expect: 'reject',
    note: 'signature was issued for /tmp/x — this is why hashCanonical(input) is in the payload' },

  { name: 'v1-tampered-toolname', version: 'v1', secret: SECRET_A,
    approvalId: base.approvalId, toolCallId: base.toolCallId, toolName: 'sendEmail',
    input, signature: goodV1, expect: 'reject' },

  { name: 'v1-swapped-approval-and-call-id', version: 'v1', secret: SECRET_A,
    approvalId: base.toolCallId, toolCallId: base.approvalId, toolName: base.toolName,
    input, signature: goodV1, expect: 'reject',
    note: 'the JSON array payload keeps field boundaries unambiguous' },

  { name: 'v1-wrong-secret', version: 'v1', secret: SECRET_B, ...base, input,
    signature: goodV1, expect: 'reject', note: 'key isolation' },

  { name: 'v1-empty-signature', version: 'v1', secret: SECRET_A, ...base, input,
    signature: '', expect: 'reject' },

  { name: 'v1-garbage-signature', version: 'v1', secret: SECRET_A, ...base, input,
    signature: '!!!not-base64!!!', expect: 'reject',
    note: 'must return false, never an error a caller could mistake for success' },

  { name: 'v1-truncated-signature', version: 'v1', secret: SECRET_A, ...base, input,
    signature: goodV1.slice(0, 20), expect: 'reject' },

  { name: 'v1-oversized-signature', version: 'v1', secret: SECRET_A, ...base, input,
    signature: goodV1 + goodV1, expect: 'reject' },

  { name: 'v1-legacy-newline-payload-must-not-verify', version: 'v1', secret: SECRET_A,
    ...base, input,
    signature: toBase64url(createHmac('sha256', SECRET_A).update(enc.encode(
      `${base.approvalId}\n${base.toolCallId}\n${base.toolName}\n${hashCanonical(input)}`,
    )).digest()),
    expect: 'reject',
    note: 'ai-go deliberately drops the legacy fallback the reference still accepts' },

  { name: 'v1-unicode-toolname', version: 'v1', secret: SECRET_A,
    approvalId: 'appr_✓', toolCallId: 'call_日本', toolName: 'xoá_tệp',
    input: { 'đường_dẫn': '/tmp/x' },
    signature: signV1(SECRET_A, 'appr_✓', 'call_日本', 'xoá_tệp', { 'đường_dẫn': '/tmp/x' }),
    expect: 'accept' },

  { name: 'v1-newline-in-fields', version: 'v1', secret: SECRET_A,
    approvalId: 'a\n1', toolCallId: 'c\n1', toolName: 'del\nete',
    input, signature: signV1(SECRET_A, 'a\n1', 'c\n1', 'del\nete', input),
    expect: 'accept',
    note: 'the JSON payload is injective, so newlines in fields are fine' },

  { name: 'v1-null-input', version: 'v1', secret: SECRET_A, ...base, input: null,
    signature: signV1(SECRET_A, base.approvalId, base.toolCallId, base.toolName, null),
    expect: 'accept' },

  // --- v2: ai-go's own payload. TS has no v2, so these are generated here from the
  //     documented format and Go must match; that keeps the check bidirectional.
  { name: 'v2-valid', version: 'v2', secret: SECRET_A, ...base, input, ...P,
    signature: goodV2, expect: 'accept' },

  { name: 'v2-tampered-principal', version: 'v2', secret: SECRET_A, ...base, input,
    principalId: 'user_99', chatId: P.chatId, iat: P.iat,
    signature: goodV2, expect: 'reject',
    note: 'the term v1 lacks: a signature cannot be replayed as another principal' },

  { name: 'v2-tampered-chat', version: 'v2', secret: SECRET_A, ...base, input,
    principalId: P.principalId, chatId: 'chat_other', iat: P.iat,
    signature: goodV2, expect: 'reject',
    note: 'cross-chat replay' },

  { name: 'v2-tampered-iat', version: 'v2', secret: SECRET_A, ...base, input,
    principalId: P.principalId, chatId: P.chatId, iat: P.iat + 1,
    signature: goodV2, expect: 'reject' },

  { name: 'v2-tampered-input', version: 'v2', secret: SECRET_A, ...base,
    input: { path: '/etc/passwd', force: false }, ...P,
    signature: goodV2, expect: 'reject' },

  { name: 'v2-wrong-secret', version: 'v2', secret: SECRET_B, ...base, input, ...P,
    signature: goodV2, expect: 'reject' },

  // A v1 signature presented to a v2-emitting server must still verify, so a TS server
  // in the same deployment stays interoperable one-directionally.
  { name: 'v1-signature-accepted-by-v2-verifier', version: 'v1-via-v2', secret: SECRET_A,
    ...base, input, ...P, signature: goodV1, expect: 'accept',
    note: 'interop: verify accepts v2 first, then falls back to v1' },
];

writeFileSync(`${OUT}canonical-vectors.json`, JSON.stringify(canonicalVectors, null, 2) + '\n');
writeFileSync(`${OUT}approval-vectors.json`, JSON.stringify(approvalVectors, null, 2) + '\n');

console.log(`wrote ${canonicalVectors.length} canonical + ${approvalVectors.length} approval vectors to ${OUT}`);
