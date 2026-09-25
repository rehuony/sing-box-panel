import { afterEach, describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

afterEach(() => vi.useRealTimers());

function fixture() {
  const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
    new Response(JSON.stringify({ items: [], nodes: [] }), { status: 200 }));
  return { client: createHttpApiClient({ fetcher }), fetcher };
}

describe('navigation data caching', () => {
  it('reuses metadata on revisits but reads runtime and configuration again', async () => {
    const { client, fetcher } = fixture();
    for (let visit = 0; visit < 2; visit++) {
      await Promise.all([
        client.getSystemStatus(), client.listCoreArtifacts(), client.listCatalogAssets(),
        client.getConfigurationSchema('core_1'), client.getSubscriptionNodeCatalog(),
        client.listSubscriptionSources(), client.listSubscriptionChannels(), client.listSubscriptionUsers(),
        client.getRuntimeStatus(), client.getConfigurationFile(),
      ]);
    }
    expect(fetcher).toHaveBeenCalledTimes(12);
    expect(fetcher.mock.calls.filter(([url]) => String(url).endsWith('/core/status'))).toHaveLength(2);
    expect(fetcher.mock.calls.filter(([url]) => String(url).endsWith('/config/file'))).toHaveLength(2);
  });

  it('expires lists and keeps different filters, artifacts and clients separate', async () => {
    vi.useFakeTimers();
    const { client, fetcher } = fixture();
    await client.listCoreArtifacts({ architecture: 'amd64' });
    await client.listCoreArtifacts({ architecture: 'arm64' });
    await client.getConfigurationSchema('core_1');
    await client.getConfigurationSchema('core_2');
    await createHttpApiClient({ fetcher }).getConfigurationSchema('core_1');
    await vi.advanceTimersByTimeAsync(15_000);
    await client.listCoreArtifacts({ architecture: 'amd64' });
    expect(fetcher).toHaveBeenCalledTimes(6);
  });

  it.each(['write', 'refresh', 'login', 'logout', 'unauthorized', 'failed write'] as const)(
    'invalidates completed reads after %s', async action => {
      const { client, fetcher } = fixture();
      await client.getConfigurationSchema('core_1');
      switch (action) {
        case 'write': await client.enableCore('core_1');
          break;
        case 'refresh': client.invalidateReadCache();
          break;
        case 'login': await client.login('test-token');
          break;
        case 'logout': await client.logout();
          break;
        case 'unauthorized':
          fetcher.mockResolvedValueOnce(new Response('{}', { status: 401 }));
          await expect(client.getRuntimeStatus()).rejects.toMatchObject({ status: 401 });
          break;
        case 'failed write':
          fetcher.mockRejectedValueOnce(new TypeError('connection closed'));
          await expect(client.enableCore('core_1')).rejects.toThrow('connection closed');
          break;
      }
      await client.getConfigurationSchema('core_1');
      expect(fetcher.mock.calls.filter(([url]) => String(url).endsWith('/configuration-schema'))).toHaveLength(2);
    },
  );
});
