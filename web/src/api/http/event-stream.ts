import { ApiRequestError } from '../api-client';

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
  let buffer = '';
  try {
    while (true) {
      const { done, value } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      if (buffer.length > 1_048_576) throw new Error('Event stream frame exceeds the limit.');
      let boundary = /\r?\n\r?\n/.exec(buffer);
      while (boundary) {
        const frame = buffer.slice(0, boundary.index);
        buffer = buffer.slice(boundary.index + boundary[0].length);
        const lines = frame.split(/\r?\n/);
        const event = lines
          .find((line) => line.startsWith('event:'))
          ?.slice(6)
          .trim();
        if (event === expectedType) {
          const data = lines
            .filter((line) => line.startsWith('data:'))
            .map((line) => line.slice(5).trimStart())
            .join('\n');
          yield JSON.parse(data) as T;
        }
        boundary = /\r?\n\r?\n/.exec(buffer);
      }
      if (done) return;
    }
  } finally {
    await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}
