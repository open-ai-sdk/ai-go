import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { once } from 'node:events';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

export type GoServer = {
  url: string;
  stop: () => Promise<void>;
};

/**
 * Builds and starts examples/chat-server on an ephemeral port. The server
 * prints `LISTEN <url>` once it is accepting connections.
 */
export async function startGoServer(): Promise<GoServer> {
  const dir = mkdtempSync(join(tmpdir(), 'ai-go-conformance-'));
  const binary = join(dir, 'server');
  const cwd = new URL('..', import.meta.url);

  execFileSync('go', ['build', '-o', binary, '../examples/chat-server'], { cwd });

  const child: ChildProcess = spawn(binary, ['-addr', '127.0.0.1:0'], {
    cwd,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  const url = await new Promise<string>((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('Go server did not start')), 60_000);
    child.once('exit', (code) => {
      clearTimeout(timeout);
      reject(new Error(`Go server exited with code ${code}`));
    });
    child.stderr!.on('data', (data) => process.stderr.write(data));
    child.stdout!.on('data', (data) => {
      const match = String(data).match(/LISTEN (http:\/\/\S+)/);
      if (match) {
        clearTimeout(timeout);
        resolve(match[1]);
      }
    });
  });

  return {
    url,
    stop: async () => {
      if (child.exitCode === null) {
        const exited = once(child, 'exit');
        child.kill();
        await exited;
      }
      rmSync(dir, { recursive: true, force: true });
    },
  };
}

export type SSEEvent = Record<string, unknown>;

/**
 * Parses an AG-UI SSE body. AG-UI never sets an `event:` field and has no
 * [DONE] sentinel, so every frame is a bare `data:` line carrying one event.
 */
export function parseAGUIStream(body: string): { events: SSEEvent[]; sawDone: boolean } {
  const events: SSEEvent[] = [];
  let sawDone = false;

  for (const line of body.split(/\r?\n/)) {
    if (!line.startsWith('data:')) continue;
    const payload = line.slice('data:'.length).replace(/^ /, '');
    if (payload === '[DONE]') {
      sawDone = true;
      continue;
    }
    events.push(JSON.parse(payload) as SSEEvent);
  }

  return { events, sawDone };
}

export function eventTypes(events: SSEEvent[]): string[] {
  return events.map((event) => String(event.type));
}
