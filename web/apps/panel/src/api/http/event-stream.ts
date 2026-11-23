import { ApiRequestError } from '../api-client';

export class StreamRotation extends Error {}

const defaultFrameByteLimit = 1_048_576;
const LF = 0x0A;
const CR = 0x0D;

/** Bounded SSE decoder shared by telemetry and raw-output transports: the data of every event of one type. */
export async function* readJSONEvents<T>(
  response: Response,
  expectedType: string,
  frameByteLimit = defaultFrameByteLimit,
): AsyncGenerator<T> {
  for await (const { data } of readNamedJSONEvents<T>(response, [expectedType], frameByteLimit)) yield data;
}

/**
 * The received bytes that no complete frame has taken yet. It grows by
 * doubling and drops taken frames once per chunk, so each byte is copied a
 * bounded number of times however the stream splits its frames.
 */
class FrameBuffer {
  bytes = new Uint8Array(0);
  /** Where the unfinished frame starts. */
  start = 0;
  end = 0;

  append(chunk: Uint8Array) {
    if (this.start > 0) {
      this.bytes.copyWithin(0, this.start, this.end);
      this.end -= this.start;
      this.start = 0;
    }
    if (this.end + chunk.byteLength > this.bytes.byteLength) {
      const grown = new Uint8Array(Math.max(this.end + chunk.byteLength, this.bytes.byteLength * 2));
      grown.set(this.bytes.subarray(0, this.end));
      this.bytes = grown;
    }
    this.bytes.set(chunk, this.end);
    this.end += chunk.byteLength;
  }
}

/**
 * Finds the first frame boundary, a blank line (`\r?\n\r?\n`), at or after
 * `from`. It answers where the frame's text ends and where the next frame
 * starts, or, without one, where the search resumes once more bytes arrive.
 */
function findBoundary(
  { bytes, end, start }: FrameBuffer,
  from: number,
): { frameEnd: number; textEnd: number } | { resume: number } {
  for (let index = from; index < end; index++) {
    if (bytes[index] !== LF) continue;
    let next = index + 1;
    if (next < end && bytes[next] === CR) next++;
    if (next >= end) return { resume: index };
    if (bytes[next] !== LF) continue;
    const textEnd = index > start && bytes[index - 1] === CR ? index - 1 : index;
    return { frameEnd: next + 1, textEnd };
  }
  return { resume: end };
}

/**
 * The same decoder for a stream of several event types, each event with its
 * type. It scans each received byte for frame boundaries once and decodes
 * each complete frame once; a boundary is ASCII, so it never splits a
 * character.
 */
export async function* readNamedJSONEvents<T>(
  response: Response,
  expectedTypes: readonly string[],
  frameByteLimit = defaultFrameByteLimit,
): AsyncGenerator<{ event: string; data: T }> {
  if (!response.headers.get('Content-Type')?.startsWith('text/event-stream') || !response.body) {
    throw new ApiRequestError('The endpoint did not return an event stream.', {
      code: 'stream_invalid',
      status: 200,
    });
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const buffer = new FrameBuffer();
  let scan = 0;
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
      if (value) {
        scan -= buffer.start;
        buffer.append(value);
      }
      while (true) {
        const boundary = findBoundary(buffer, scan);
        if ('resume' in boundary) {
          scan = boundary.resume;
          break;
        }
        if (boundary.frameEnd - buffer.start > frameByteLimit) {
          throw new Error('Event stream frame exceeds the limit.');
        }
        const frame = decoder.decode(buffer.bytes.subarray(buffer.start, boundary.textEnd));
        buffer.start = boundary.frameEnd;
        scan = boundary.frameEnd;
        const lines = frame.split(/\r?\n/);
        const event = lines
          .find((line) => line.startsWith('event:'))
          ?.slice(6)
          .trim();
        if (event === 'control') {
          const data = lines.filter(line => line.startsWith('data:')).map(line => line.slice(5)).join('\n');
          if (JSON.parse(data).type === 'reconnect') throw new StreamRotation();
        }
        if (event !== undefined && expectedTypes.includes(event)) {
          const data = lines
            .filter((line) => line.startsWith('data:'))
            .map((line) => line.slice(5).trimStart())
            .join('\n');
          yield { data: JSON.parse(data) as T, event };
        }
      }
      if (buffer.end - buffer.start > frameByteLimit) {
        throw new Error('Event stream frame exceeds the limit.');
      }
      if (done) return;
    }
  } finally {
    await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}
