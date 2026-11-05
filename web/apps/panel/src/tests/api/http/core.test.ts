import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient core and configuration domain', () => {
  it.each(['enable', 'disable'] as const)('%s targets the selected artifact with an empty body and encoded identity', async (action) => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ observation_state: 'stopped', desired_running: false }), { status: 200 }));
    const client = createHttpApiClient({ baseUrl: '/api/v1', fetcher }).forNode('node_a');
    await client[`${action}Core`]('core/one');
    expect(fetcher).toHaveBeenCalledWith(`/api/v1/nodes/node_a/core/artifacts/core%2Fone/${action}`, expect.objectContaining({ method: 'POST' }));
    expect(fetcher.mock.calls[0][1]?.body).toBeUndefined();
  });
  it('compiles raw configuration for an immutable core artifact', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ supported: true, items: [] }), {
        status: 200, headers: { 'Content-Type': 'application/json' },
      }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher }).forNode('node_a');

    await client.compileConfiguration({ coreArtifactID: 'core_1' });

    expect(fetcher).toHaveBeenCalledWith('/panel/api/v1/nodes/node_a/config/compile', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ core_artifact_id: 'core_1' }),
    }));
  });

  it('exposes every local runtime lifecycle operation without a manual configuration path', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ observation_state: 'stopped', desired_running: false }), {
        status: 200, headers: { 'Content-Type': 'application/json' },
      }));
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    await client.startRuntime();
    await client.stopRuntime();
    await client.restartRuntime();
    await client.rollbackRuntime('bundle_rollback');

    expect(fetcher.mock.calls.map((call) => call[0])).toEqual([
      '/api/v1/nodes/node_a/core/start', '/api/v1/nodes/node_a/core/stop', '/api/v1/nodes/node_a/core/restart', '/api/v1/nodes/node_a/core/rollback',
    ]);
    for (const call of fetcher.mock.calls) expect(call[1]).toEqual(expect.objectContaining({ method: 'POST' }));
    expect(fetcher).toHaveBeenNthCalledWith(4, '/api/v1/nodes/node_a/core/rollback', expect.objectContaining({
      body: JSON.stringify({ activation_bundle_id: 'bundle_rollback' }),
    }));
  });

  it('loads an exact reviewed schema by encoded core artifact', async () => {
    const schema = {
      exact_version: '1.14.0',
      schema_sha256: 'a'.repeat(64),
      schema: { type: 'object' },
    };
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify(schema), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
          'ETag': `"${'a'.repeat(64)}"`,
        },
      }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher }).forNode('node_a');

    await expect(client.getConfigurationSchema('core/one')).resolves.toEqual(schema);
    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/nodes/node_a/core/artifacts/core%2Fone/configuration-schema',
      expect.objectContaining({ method: 'GET' }),
    );
  });
});
