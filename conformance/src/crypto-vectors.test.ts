import { createHash, createHmac } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// The TS side of the shared approval-crypto vectors.
//
// This half is not optional, and not redundant with the Go tests. The Go side proves Go
// reproduces these bytes; only this side proves the VECTORS THEMSELVES are right. If the
// generator had a bug, Go would faithfully match a wrong expectation and both suites would
// be green — and the specific failure that hides behind that is Go accepting a signature
// the real client would reject, which is a security hole rather than a compatibility bug.
//
// So the algorithm below is transcribed independently from the reference source rather
// than imported from the generator.

const TESTDATA = new URL('../../aisdk/testdata/', import.meta.url).pathname;
const enc = new TextEncoder();

function load<T>(name: string): T[] {
  const rows = JSON.parse(readFileSync(`${TESTDATA}${name}`, 'utf8'));
  expect(rows.length, `${name} is empty`).toBeGreaterThan(0);
  return rows;
}

// --- transcribed from ai/src/util/canonical-hash.ts ---

function canonicalJSON(value: unknown): string {
  if (value === null || value === undefined) return JSON.stringify(value);
  if (typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`;
  const keys = Object.keys(value as Record<string, unknown>).sort();
  return `{${keys
    .map(k => `${JSON.stringify(k)}:${canonicalJSON((value as Record<string, unknown>)[k])}`)
    .join(',')}}`;
}

function toBase64url(buf: Buffer | Uint8Array): string {
  return Buffer.from(buf).toString('base64')
    .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function hashCanonical(value: unknown): string {
  return toBase64url(createHash('sha256').update(enc.encode(canonicalJSON(value))).digest());
}

// --- transcribed from ai/src/generate-text/tool-approval-signature.ts, plus ai-go's v2 ---

const V1 = 'ai-sdk-tool-approval-v1';
const V2 = 'ai-go-tool-approval-v2';

interface Row {
  name: string;
  note?: string;
  version: string;
  secret: string;
  approvalId: string;
  toolCallId: string;
  toolName: string;
  input: unknown;
  signature: string;
  expect: 'accept' | 'reject';
  principalId?: string;
  chatId?: string;
  iat?: number;
}

function macV1(r: Row): string {
  const payload = JSON.stringify([V1, r.approvalId, r.toolCallId, r.toolName, hashCanonical(r.input)]);
  return toBase64url(createHmac('sha256', r.secret).update(enc.encode(payload)).digest());
}

function macV2(r: Row): string {
  const payload = JSON.stringify([
    V2, r.approvalId, r.toolCallId, r.toolName, hashCanonical(r.input),
    r.principalId ?? '', r.chatId ?? '', r.iat ?? 0,
  ]);
  return toBase64url(createHmac('sha256', r.secret).update(enc.encode(payload)).digest());
}

/**
 * Decodes a signature the way the reference client does: rewrite the url alphabet, then
 * atob — which tolerates padding, the standard alphabet, and an unpadded final group.
 * Anything undecodable is a rejection, never an exception the caller could mishandle.
 */
function decodeTolerant(sig: string): Buffer | null {
  try {
    const std = sig.replace(/-/g, '+').replace(/_/g, '/');
    const buf = Buffer.from(std, 'base64');
    // Buffer.from is lenient to the point of silently truncating garbage, so reject
    // anything that did not round-trip to at least one byte.
    return buf.length > 0 ? buf : null;
  } catch {
    return null;
  }
}

/** Mirrors ai-go's VerifyToolApproval: try v2, then v1, constant-time compare. */
function verify(r: Row): boolean {
  const provided = decodeTolerant(r.signature);
  if (provided == null) return false;

  for (const expected of [macV2(r), macV1(r)]) {
    const want = decodeTolerant(expected);
    if (want != null && want.length === provided.length && want.equals(provided)) {
      return true;
    }
  }
  return false;
}

describe('canonical vectors', () => {
  const rows = load<{
    name: string; input: unknown; canonicalJSON: string; hashCanonical: string;
  }>('canonical-vectors.json');

  for (const r of rows) {
    it(`${r.name} canonicalizes and hashes as recorded`, () => {
      expect(canonicalJSON(r.input)).toBe(r.canonicalJSON);
      expect(hashCanonical(r.input)).toBe(r.hashCanonical);
    });
  }

  // The vector that detects the Go/JS key-sort divergence has to be present AND has to
  // encode UTF-16 order. Without this guard the fixture set could quietly lose the only
  // case that distinguishes the two sorts.
  it('the sort-divergence vector encodes UTF-16 order, not byte order', () => {
    const r = rows.find(x => x.name === 'nonbmp-key-vs-private-use-key');
    expect(r, 'the non-BMP vs private-use-key vector is missing').toBeDefined();

    const emoji = r!.canonicalJSON.indexOf('\u{1F600}');
    const priv = r!.canonicalJSON.indexOf('');
    expect(emoji, 'lost the non-BMP key').toBeGreaterThanOrEqual(0);
    expect(priv, 'lost the private-use key').toBeGreaterThanOrEqual(0);
    // U+1F600's first code unit is 0xD83D, below U+E000, so UTF-16 order puts it first.
    // UTF-8 byte order would put U+E000 (0xEE…) before U+1F600 (0xF0…).
    expect(emoji).toBeLessThan(priv);
  });

  it('a value containing < > & is not escaped', () => {
    const r = rows.find(x => x.name === 'html-chars-in-value');
    expect(r).toBeDefined();
    expect(r!.canonicalJSON).toContain('a < b && c > d');
    expect(r!.canonicalJSON).not.toContain('\\u003c');
    expect(r!.canonicalJSON).not.toContain('&amp;amp;');
  });
});

describe('approval vectors', () => {
  const rows = load<Row>('approval-vectors.json');

  for (const r of rows) {
    it(`${r.name} → ${r.expect}`, () => {
      expect(verify(r), r.note ?? '').toBe(r.expect === 'accept');
    });
  }

  it('exercises both outcomes', () => {
    const accepts = rows.filter(r => r.expect === 'accept').length;
    const rejects = rows.filter(r => r.expect === 'reject').length;
    expect(accepts, 'no accept vectors').toBeGreaterThan(0);
    expect(rejects, 'no reject vectors').toBeGreaterThan(0);
  });

  it('every recorded v1 accept signature is the one the reference would produce', () => {
    // Catches a generator that recorded a signature Go happens to reproduce but the real
    // reference would not.
    const v1 = rows.filter(r => r.version === 'v1' && r.expect === 'accept'
      && !/[+/=]/.test(r.signature));
    expect(v1.length).toBeGreaterThan(0);
    for (const r of v1) {
      expect(r.signature, r.name).toBe(macV1(r));
    }
  });

  it('the padded and standard-base64 variants decode to the same MAC', () => {
    for (const name of ['v1-valid-padded-signature', 'v1-valid-standard-base64-signature']) {
      const r = rows.find(x => x.name === name);
      expect(r, `${name} is missing`).toBeDefined();
      const provided = decodeTolerant(r!.signature);
      const expected = decodeTolerant(macV1(r!));
      expect(provided, name).not.toBeNull();
      expect(provided!.equals(expected!), name).toBe(true);
    }
  });

  it('the legacy newline payload is recorded as a rejection', () => {
    // The reference ACCEPTS this format as a verify-time fallback; ai-go deliberately
    // does not. That divergence is intentional and narrows what the server trusts, so
    // the vector must stay a reject.
    const r = rows.find(x => x.name === 'v1-legacy-newline-payload-must-not-verify');
    expect(r, 'the legacy-newline vector is missing').toBeDefined();
    expect(r!.expect).toBe('reject');

    // And confirm it really is a legacy-format signature rather than random bytes —
    // otherwise the vector would pass for the wrong reason.
    const legacy = toBase64url(createHmac('sha256', r!.secret).update(enc.encode(
      `${r!.approvalId}\n${r!.toolCallId}\n${r!.toolName}\n${hashCanonical(r!.input)}`,
    )).digest());
    expect(r!.signature).toBe(legacy);
  });

  it('v2 binds principal, chat, and issuance time', () => {
    // Each of these is a term v1 lacks, and each tampered variant must reject.
    for (const name of ['v2-tampered-principal', 'v2-tampered-chat', 'v2-tampered-iat']) {
      const r = rows.find(x => x.name === name);
      expect(r, `${name} is missing`).toBeDefined();
      expect(r!.expect, name).toBe('reject');
      expect(verify(r!), name).toBe(false);
    }
  });

  it('a v1 signature is accepted by the v2 verifier', () => {
    const r = rows.find(x => x.name === 'v1-signature-accepted-by-v2-verifier');
    expect(r, 'the interop vector is missing').toBeDefined();
    expect(verify(r!)).toBe(true);
  });
});
