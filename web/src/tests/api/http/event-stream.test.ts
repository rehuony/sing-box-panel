import { describe, expect, it } from 'vitest';

import { readJSONEvents } from '@/api/http/event-stream';

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
});
