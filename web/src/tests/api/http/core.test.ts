import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient core and adapter domain', () => {
  it('resolves support by immutable core artifact and compiles accepted projection evidence', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ supported: true, items: [] }), {
        status: 200, headers: { 'Content-Type': 'application/json' },
      }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await client.getConfigurationSupport('core/one');
    await client.previewConfiguration({ coreArtifactID: 'core_1', canonicalRevisionID: 'revision_3' });
    await client.compileConfiguration({ coreArtifactID: 'core_1', acceptedIgnoredDigest: 'a'.repeat(64) });

    expect(fetcher.mock.calls[0]?.[0]).toBe('/panel/api/v1/core/artifacts/core%2Fone/configuration-support');
    expect(fetcher).toHaveBeenNthCalledWith(2, '/panel/api/v1/config/preview', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ core_artifact_id: 'core_1', canonical_revision_id: 'revision_3' }),
    }));
    expect(fetcher).toHaveBeenNthCalledWith(3, '/panel/api/v1/config/compile', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ core_artifact_id: 'core_1', accepted_ignored_digest: 'a'.repeat(64) }),
    }));
  });

  it('exposes every local runtime lifecycle operation without a manual configuration path', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ id: 'task_1' }), {
        status: 200, headers: { 'Content-Type': 'application/json' },
      }));
    const client = createHttpApiClient({ fetcher });
    await client.startRuntime();
    await client.stopRuntime();
    await client.restartRuntime();
    await client.rollbackRuntime();

    expect(fetcher.mock.calls.map((call) => call[0])).toEqual([
      '/api/v1/core/start', '/api/v1/core/stop', '/api/v1/core/restart', '/api/v1/core/rollback',
    ]);
    for (const call of fetcher.mock.calls) expect(call[1]).toEqual(expect.objectContaining({ method: 'POST' }));
  });
});
