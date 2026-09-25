import { afterEach, describe, expect, it, vi } from 'vitest';

import { createReadCache } from '@/api/http/read-cache';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => {
    resolve = done;
  });
  return { promise, resolve };
}

afterEach(() => vi.useRealTimers());

describe('hTTP read cache', () => {
  it('deduplicates pending reads, isolates returned values and expires completed reads', async () => {
    vi.useFakeTimers();
    const cache = createReadCache();
    const pending = deferred<{ items: string[] }>();
    const load = vi.fn(() => pending.promise);
    const first = cache.read('/items', load, 1000);
    const second = cache.read('/items', load, 1000);
    pending.resolve({ items: ['one'] });
    const value = await first;
    value.items.push('local edit');
    expect(await second).toEqual({ items: ['one'] });
    expect(await cache.read('/items', load, 1000)).toEqual({ items: ['one'] });
    expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1000);
    await cache.read('/items', load, 1000);
    expect(load).toHaveBeenCalledTimes(2);
  });

  it('lets one reader abort without cancelling another reader', async () => {
    const cache = createReadCache();
    const pending = deferred<number>();
    const load = vi.fn((_signal: AbortSignal) => pending.promise);
    const controller = new AbortController();
    const first = cache.read('/items', load, 1000, controller.signal);
    const second = cache.read('/items', load, 1000);
    const cancelled = expect(first).rejects.toMatchObject({ name: 'AbortError' });
    controller.abort();
    await cancelled;
    expect(load.mock.calls[0][0].aborted).toBe(false);
    pending.resolve(42);
    expect(await second).toBe(42);
    expect(load).toHaveBeenCalledTimes(1);
  });

  it('cancels an abandoned request and allows a fresh caller to retry immediately', async () => {
    const cache = createReadCache();
    const pending = deferred<number>();
    const load = vi.fn((_signal: AbortSignal) => pending.promise);
    const controller = new AbortController();
    const first = cache.read('/items', load, 1000, controller.signal);
    const cancelled = expect(first).rejects.toMatchObject({ name: 'AbortError' });
    await Promise.resolve();
    controller.abort();
    await cancelled;
    expect(load.mock.calls[0][0].aborted).toBe(true);
    const retry = cache.read('/items', load, 1000);
    pending.resolve(42);
    expect(await retry).toBe(42);
    expect(load).toHaveBeenCalledTimes(2);
  });

  it('does not revive a stale in-flight result after invalidation', async () => {
    const cache = createReadCache();
    const pending = deferred<number>();
    const load = vi.fn().mockReturnValueOnce(pending.promise).mockResolvedValue(2);
    const old = cache.read('/items', load, 1000);
    await Promise.resolve();
    cache.clear();
    expect(await cache.read('/items', load, 1000)).toBe(2);
    pending.resolve(1);
    expect(await old).toBe(1);
    expect(await cache.read('/items', load, 1000)).toBe(2);
    expect(load).toHaveBeenCalledTimes(2);
  });

  it('retries errors and only deduplicates in-flight reads with zero lifetime', async () => {
    const cache = createReadCache();
    const load = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(2);
    await expect(cache.read('/items', load, 1000)).rejects.toThrow('offline');
    expect(await cache.read('/items', load, 0)).toBe(2);
    expect(await cache.read('/items', load, 0)).toBe(2);
    expect(load).toHaveBeenCalledTimes(3);
  });

  it('does not start reads for an already aborted consumer', async () => {
    const load = vi.fn();
    await expect(createReadCache().read('/items', load, 1000, AbortSignal.abort()))
      .rejects
      .toMatchObject({ name: 'AbortError' });
    expect(load).not.toHaveBeenCalled();
  });
});
