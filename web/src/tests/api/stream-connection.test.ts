import { describe, expect, it, vi } from 'vitest';

import { ApiRequestError } from '@/api/api-client';
import { followStream } from '@/api/stream-connection';
import { StreamRotation } from '@/api/http/event-stream';

describe('stream reconnection', () => {
  it('renews for 30 minutes of virtual time without raising an outage or losing cancellation', async () => {
    vi.useFakeTimers();
    const received = vi.fn();
    const failed = vi.fn();
    const gap = vi.fn();
    let attempts = 0;
    let active = 0;
    const stop = followStream(async function* (signal) {
      attempts++;
      active++;
      try {
        yield attempts;
        await new Promise<void>(resolve => {
          const timer = setTimeout(resolve, 59_000);
          signal.addEventListener('abort', () => {
            clearTimeout(timer);
            resolve();
          }, { once: true });
        });
        throw new StreamRotation();
      } finally {
        active--;
      }
    }, received, failed, gap);
    try {
      await vi.advanceTimersByTimeAsync(30 * 60_000);
      expect(attempts).toBe(31);
      expect(received).toHaveBeenCalledTimes(31);
      expect(active).toBe(1);
      expect(failed).not.toHaveBeenCalled();
      expect(gap).not.toHaveBeenCalled();
      stop();
      await vi.advanceTimersByTimeAsync(0);
      expect(active).toBe(0);
    } finally {
      stop();
      vi.useRealTimers();
    }
  });

  it('allows a short recovery grace and stops retrying unauthorized connections', async () => {
    vi.useFakeTimers();
    const failed = vi.fn();
    const received = vi.fn();
    let attempts = 0;
    const stop = followStream(async function* () {
      if (++attempts === 1) throw new TypeError('offline');
      yield 42;
      throw new ApiRequestError('expired', { status: 401, code: 'unauthorized' });
    }, received, failed, () => {});
    try {
      await vi.advanceTimersByTimeAsync(60_000);
      expect(attempts).toBe(2);
      expect(received).toHaveBeenCalledExactlyOnceWith(42);
      expect(failed).not.toHaveBeenCalled();
    } finally {
      stop();
      vi.useRealTimers();
    }
  });
});
