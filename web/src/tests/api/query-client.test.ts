import { describe, expect, it, vi } from 'vitest';

import type { ApiClient } from '@/api/api-client';

import { queries } from '@/api/queries';
import { deferred } from '@/tests/deferred';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { createQueryClient, resetQuerySession, withQueryInvalidation } from '@/api/query-client';

describe('session query cache', () => {
  it('deduplicates navigation reads and keeps resource invalidation local', async () => {
    const cache = createQueryClient();
    const raw = createMockApiClient();
    const client = withQueryInvalidation(raw, cache);
    try {
      await Promise.all([
        cache.fetchQuery(queries.sources(client)), cache.fetchQuery(queries.sources(client)),
        cache.fetchQuery(queries.system(client)),
      ]);
      expect(raw.listSubscriptionSources).toHaveBeenCalledOnce();
      await client.refreshSubscriptionSource('one');
      await Promise.all([cache.fetchQuery(queries.sources(client)), cache.fetchQuery(queries.system(client))]);
      expect(raw.listSubscriptionSources).toHaveBeenCalledTimes(2);
      expect(raw.getSystemStatus).toHaveBeenCalledOnce();
    } finally {
      cache.clear();
    }
  });

  it('cancels old reads before a write and invalidates even an ambiguous failure', async () => {
    const cache = createQueryClient();
    const raw = createMockApiClient();
    const client = withQueryInvalidation(raw, cache);
    let captured: AbortSignal | undefined;
    raw.getSubscriptionNodeCatalog.mockImplementationOnce(signal => {
      captured = signal;
      return new Promise(() => {});
    });
    raw.refreshSubscriptionSource.mockRejectedValueOnce(new TypeError('connection closed'));
    const read = cache.fetchQuery(queries.nodes(client)).catch(() => undefined);
    await vi.waitFor(() => expect(captured).toBeDefined());
    await expect(client.refreshSubscriptionSource('one')).rejects.toThrow('connection closed');
    await read;
    expect(captured?.aborted).toBe(true);
    await cache.fetchQuery(queries.nodes(client));
    expect(raw.getSubscriptionNodeCatalog).toHaveBeenCalledTimes(2);
    cache.clear();
  });

  it('clears all retained data on a session change', async () => {
    const cache = createQueryClient();
    const raw = createMockApiClient();
    const client = withQueryInvalidation(raw, cache);
    await cache.fetchQuery(queries.system(client));
    await client.logout();
    expect(cache.getQueryCache().getAll()).toHaveLength(0);
    cache.clear();
  });
  it('rejects a write response from an expired session before it can restore old data', async () => {
    const pending = deferred<Awaited<ReturnType<ApiClient['savePanelSettings']>>>();
    const raw = createMockApiClient({ savePanelSettings: vi.fn(() => pending.promise) });
    const cache = createQueryClient();
    const client = withQueryInvalidation(raw, cache);
    const saved = client.savePanelSettings({} as Parameters<ApiClient['savePanelSettings']>[0]);
    const rejection = expect(saved).rejects.toMatchObject({ name: 'AbortError' });
    await Promise.resolve();
    resetQuerySession(cache);
    pending.resolve({ settings: await raw.getPanelSettings(), reauthentication_required: false });
    await rejection;
    expect(cache.getQueryCache().getAll()).toEqual([]);
  });
});
