import { ApiRequestError } from '../api-client';

export class StreamRotation extends Error {}

const frameByteLimit = 1_048_576;

/** Bounded SSE decoder shared by telemetry and raw-output transports. */
export async function* readJSONEvents<T>(
  response: Response,
  expectedType: string,
): AsyncGenerator<T> {
  if (!response.headers.get('Content-Type')?.startsWith('text/event-stream') || !response.body) {
    throw new ApiRequestError('The endpoint did not return an event stream.', {
      code: 'stream_invalid',
      status: 200,
    });
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const encoder = new TextEncoder();
  let buffer = '';
  try {
    while (true) {
      let idle = false;
      const timer = setTimeout(() => {
        idle = true;
        void reader.cancel();
      }, 25_000);
      let chunk: ReadableStreamReadResult<Uint8Array>;
      try {
        chunk = await reader.read();
      } finally {
        clearTimeout(timer);
      }
      if (idle) throw new ApiRequestError('The event stream stopped responding.', { code: 'stream_idle', status: 0 });
      const { done, value } = chunk;
      buffer += decoder.decode(value, { stream: !done });
      let boundary = /\r?\n\r?\n/.exec(buffer);
      while (boundary) {
        const frameEnd = boundary.index + boundary[0].length;
        if (encoder.encode(buffer.slice(0, frameEnd)).byteLength > frameByteLimit) {
          throw new Error('Event stream frame exceeds the limit.');
        }
        const frame = buffer.slice(0, boundary.index);
        buffer = buffer.slice(frameEnd);
        const lines = frame.split(/\r?\n/);
        const event = lines
          .find((line) => line.startsWith('event:'))
          ?.slice(6)
          .trim();
        if (event === 'control') {
          const data = lines.filter(line => line.startsWith('data:')).map(line => line.slice(5)).join('\n');
          if (JSON.parse(data).type === 'reconnect') throw new StreamRotation();
        }
        if (event === expectedType) {
          const data = lines
            .filter((line) => line.startsWith('data:'))
            .map((line) => line.slice(5).trimStart())
            .join('\n');
          yield JSON.parse(data) as T;
        }
        boundary = /\r?\n\r?\n/.exec(buffer);
      }
      if (encoder.encode(buffer).byteLength > frameByteLimit) {
        throw new Error('Event stream frame exceeds the limit.');
      }
      if (done) return;
    }
  } finally {
    await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}
