import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { UI_MESSAGE_STREAM_HEADERS } from 'ai';
import { afterAll, beforeAll, describe, expect, it } from 'vite-plus/test';

let server: ChildProcess;
let serverURL: string;
let serverDir: string;

beforeAll(async () => {
  serverDir = mkdtempSync(join(tmpdir(), 'ai-go-conformance-'));
  const serverPath = join(serverDir, 'server');
  execFileSync('go', ['build', '-o', serverPath, '../examples/chat-server'], {
    cwd: new URL('..', import.meta.url),
  });
  server = spawn(serverPath, ['-addr', '127.0.0.1:0'], {
    cwd: new URL('..', import.meta.url),
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  serverURL = await new Promise<string>((resolve, reject) => {
    const timeout = setTimeout(
      () => reject(new Error('Go conformance server did not start')),
      60_000,
    );
    server.once('exit', (code) => {
      clearTimeout(timeout);
      reject(new Error(`Go conformance server exited with code ${code}`));
    });
    server.stderr!.on('data', (data) => process.stderr.write(data));
    server.stdout!.on('data', (data) => {
      const match = String(data).match(/LISTEN (http:\/\/\S+)/);
      if (match) {
        clearTimeout(timeout);
        resolve(match[1]);
      }
    });
  });
});

afterAll(async () => {
  if (server?.exitCode === null) {
    const exited = once(server, 'exit');
    server.kill();
    await exited;
  }
  if (serverDir) {
    rmSync(serverDir, { recursive: true, force: true });
  }
});

describe('live Go UI message stream', () => {
  async function fetchScenario(scenario: string) {
    const response = await fetch(`${serverURL}/chat?scenario=${scenario}`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        messages: [
          {
            id: 'user-1',
            role: 'user',
            parts: [{ type: 'text', text: 'hello' }],
          },
        ],
      }),
    });

    expect(response.status).toBe(200);
    for (const [name, value] of Object.entries(UI_MESSAGE_STREAM_HEADERS)) {
      expect(response.headers.get(name), name).toBe(value);
    }
    return response.text();
  }

  it('matches all five headers and terminates successful streams', async () => {
    const body = await fetchScenario('text');
    expect(body).toContain('"type":"text-delta"');
    expect(body.endsWith('data: [DONE]\n\n')).toBe(true);
  });

  it('closes blocks and terminates error streams', async () => {
    const body = await fetchScenario('error');
    expect(body).toContain('"type":"error"');
    expect(body).toContain('"type":"text-end"');
    expect(body).toContain('"finishReason":"error"');
    expect(body.endsWith('data: [DONE]\n\n')).toBe(true);
  });
});
