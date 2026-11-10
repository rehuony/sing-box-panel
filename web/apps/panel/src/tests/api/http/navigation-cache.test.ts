import { afterEach, describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

afterEach(() => vi.useRealTimers());

describe('hTTP read policy', () => {
  it('retries a transient GET once without retaining transport data', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockRejectedValueOnce(new TypeError('offline'))
      .mockImplementation(async () => new Response('{}'));
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    await client.getFleetStatus();
    await client.getFleetStatus();
    expect(fetcher).toHaveBeenCalledTimes(3);
  });
  it('does not retry writes or authorization failures', async () => {
    const fetcher = vi.fn<typeof fetch>().mockRejectedValueOnce(new TypeError('offline')).mockResolvedValueOnce(new Response('{}', { status: 401 }));
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    await expect(client.startRuntime()).rejects.toThrow('offline');
    await expect(client.getFleetStatus()).rejects.toMatchObject({ status: 401 });
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('bounds a stalled read including its response body', async () => {
    vi.useFakeTimers();
    const fetcher = vi.fn<typeof fetch>().mockImplementation((_url, init) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(init.signal?.reason));
    }));
    const request = createHttpApiClient({ fetcher }).forNode('node_a').getFleetStatus();
    const failure = expect(request).rejects.toMatchObject({ name: 'TimeoutError' });
    await vi.advanceTimersByTimeAsync(15_000);
    await failure;
    expect(fetcher).toHaveBeenCalledOnce();
  });
});
