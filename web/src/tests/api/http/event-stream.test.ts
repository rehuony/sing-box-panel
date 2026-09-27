import { describe, expect, it, vi } from 'vitest';

import { createHttpApiContext } from '@/api/http/shared';
import { readJSONEvents, StreamRotation } from '@/api/http/event-stream';

const frameLimit = 1_048_576;
const emptyFrame = 'event: dashboard\ndata: ""\n\n';

function eventResponse(chunks: string[]): Response {
  const encoder = new TextEncoder();
  return new Response(new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
      controller.close();
    },
  }), { headers: { 'Content-Type': 'text/event-stream' } });
}

function frameOfSize(bytes: number): string {
  return `event: dashboard\ndata: "${'x'.repeat(bytes - emptyFrame.length)}"\n\n`;
}

describe('readJSONEvents frame bounds', () => {
  it('also bounds connections that never return response headers', async () => {
    vi.useFakeTimers();
    const fetcher = vi.fn<typeof fetch>((_url, init) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(init.signal?.reason), { once: true });
    }));
    try {
      const request = createHttpApiContext({ fetcher }).openEventStream(fetcher, '/stream', {});
      const failure = expect(request).rejects.toMatchObject({ code: 'stream_idle' });
      await vi.advanceTimersByTimeAsync(25_000);
      await failure;
    } finally {
      vi.useRealTimers();
    }
  });

  it('accepts multiple bounded frames coalesced into one network chunk', async () => {
    const frame = frameOfSize(frameLimit);
    const iterator = readJSONEvents<string>(eventResponse([frame + frame]), 'dashboard');
    expect((await iterator.next()).value?.length).toBe(frameLimit - emptyFrame.length);
    expect((await iterator.next()).value?.length).toBe(frameLimit - emptyFrame.length);
    expect((await iterator.next()).done).toBe(true);
  });

  it('rejects a complete oversized frame and an oversized unfinished frame', async () => {
    for (const frame of [frameOfSize(frameLimit + 1), `data: ${'x'.repeat(frameLimit)}`]) {
      const iterator = readJSONEvents(eventResponse([frame.slice(0, 100), frame.slice(100)]), 'dashboard');
      await expect(iterator.next().then(() => undefined)).rejects.toThrow('Event stream frame exceeds the limit.');
    }
  });

  it('counts UTF-8 bytes rather than UTF-16 characters', async () => {
    const frame = `event: dashboard\ndata: "${'中'.repeat(400_000)}"\n\n`;
    expect(frame.length).toBeLessThan(frameLimit);
    const iterator = readJSONEvents(eventResponse([frame]), 'dashboard');
    await expect(iterator.next().then(() => undefined)).rejects.toThrow('Event stream frame exceeds the limit.');
  });
  it('recognizes intentional renewal and bounds a half-open stream at 25 seconds', async () => {
    await expect(readJSONEvents(eventResponse(['event: control\ndata: {"type":"reconnect"}\n\n']), 'metrics').next())
      .rejects
      .toBeInstanceOf(StreamRotation);
    vi.useFakeTimers();
    const canceled = vi.fn();
    const response = new Response(new ReadableStream({ cancel: canceled }), {
      headers: { 'Content-Type': 'text/event-stream' },
    });
    try {
      const next = readJSONEvents(response, 'dashboard').next();
      const failure = expect(next).rejects.toMatchObject({ code: 'stream_idle' });
      await vi.advanceTimersByTimeAsync(25_000);
      await failure;
      expect(canceled).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });
});
