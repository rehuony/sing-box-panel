import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient core and configuration domain', () => {
  it('enables the selected artifact with an empty body and encoded identity', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ id: 'switch', status: 'queued' }), { status: 202 }));
    const client = createHttpApiClient({ baseUrl: '/api/v1', fetcher });
    await client.enableCore('core/one');
    expect(fetcher).toHaveBeenCalledWith('/api/v1/core/artifacts/core%2Fone/enable', expect.objectContaining({ method: 'POST' }));
    expect(fetcher.mock.calls[0][1]?.body).toBeUndefined();
  });
  it('resolves Schema support by immutable core artifact and compiles raw configuration', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ supported: true, items: [] }), {
        status: 200, headers: { 'Content-Type': 'application/json' },
      }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await client.getConfigurationSupport('core/one');
    await client.previewConfiguration({ coreArtifactID: 'core_1', canonicalRevisionID: 'revision_3' });
    await client.compileConfiguration({ coreArtifactID: 'core_1' });

    expect(fetcher.mock.calls[0]?.[0]).toBe('/panel/api/v1/core/artifacts/core%2Fone/configuration-support');
    expect(fetcher).toHaveBeenNthCalledWith(2, '/panel/api/v1/config/preview', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ core_artifact_id: 'core_1', canonical_revision_id: 'revision_3' }),
    }));
    expect(fetcher).toHaveBeenNthCalledWith(3, '/panel/api/v1/config/compile', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ core_artifact_id: 'core_1' }),
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
    await client.rollbackRuntime('bundle_rollback');

    expect(fetcher.mock.calls.map((call) => call[0])).toEqual([
      '/api/v1/core/start', '/api/v1/core/stop', '/api/v1/core/restart', '/api/v1/core/rollback',
    ]);
    for (const call of fetcher.mock.calls) expect(call[1]).toEqual(expect.objectContaining({ method: 'POST' }));
    expect(fetcher).toHaveBeenNthCalledWith(4, '/api/v1/core/rollback', expect.objectContaining({
      body: JSON.stringify({ activation_bundle_id: 'bundle_rollback' }),
    }));
  });

  it('loads an exact reviewed schema and sends every runtime-history cursor filter', async () => {
    const schema = {
      exact_version: '1.14.0',
      schema_sha256: 'a'.repeat(64),
      schema: { type: 'object' },
    };
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async (input) =>
      new Response(JSON.stringify(String(input).includes('configuration-schema')
        ? schema
        : {
            items: [], history_started_at: '2026-08-26T00:00:00Z',
          }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
          'ETag': `"${'a'.repeat(64)}"`,
        },
      }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await expect(client.getConfigurationSchema('core/one')).resolves.toEqual(schema);
    await client.getRuntimeHistory({
      from: '2026-08-26T00:00:00Z',
      to: '2026-08-27T00:00:00Z',
      state: 'unknown',
      reason: 'heartbeat_lost',
      activationBundleID: 'bundle_18',
      beforeTime: '2026-08-26T12:00:00Z',
      beforeID: 42,
      limit: 25,
    });

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      '/panel/api/v1/core/artifacts/core%2Fone/configuration-schema',
      expect.objectContaining({ method: 'GET' }),
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      '/panel/api/v1/core/runtime/history?from=2026-08-26T00%3A00%3A00Z&to=2026-08-27T00%3A00%3A00Z&state=unknown&reason=heartbeat_lost&activation_bundle_id=bundle_18&before_time=2026-08-26T12%3A00%3A00Z&before_id=42&limit=25',
      expect.objectContaining({ method: 'GET' }),
    );
  });
});
