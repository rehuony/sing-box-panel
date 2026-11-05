import { ApiRequestError } from './api-client';
import { StreamRotation } from './http/event-stream';

/** One reconnect policy for both independent telemetry channels. */
export function followStream<T>(stream: (signal: AbortSignal) => AsyncIterable<T>, receive: (value: T) => void,
  failed: (error: unknown) => void, gap: () => void) {
  const controller = new AbortController();
  let retry: ReturnType<typeof setTimeout> | undefined;
  let grace: ReturnType<typeof setTimeout> | undefined;
  let attempt = 0;
  async function connect() {
    let rotation = false;
    try {
      for await (const value of stream(controller.signal)) {
        if (controller.signal.aborted) return;
        attempt = 0;
        clearTimeout(grace);
        grace = undefined;
        receive(value);
      }
      if (!controller.signal.aborted) throw new Error('The event stream ended unexpectedly.');
    } catch (error) {
      if (controller.signal.aborted) return;
      rotation = error instanceof StreamRotation;
      if (error instanceof ApiRequestError && error.status === 401) return;
      if (!rotation) {
        gap();
        grace ??= setTimeout(failed, 5000, error);
      }
    }
    if (!controller.signal.aborted) {
      const delay = rotation ? 0 : Math.min(30_000, 1000 * 2 ** Math.min(attempt++, 5)) * (0.75 + Math.random() * 0.25);
      retry = setTimeout(() => void connect(), delay);
    }
  }
  void connect();
  return () => {
    controller.abort();
    clearTimeout(retry);
    clearTimeout(grace);
  };
}
