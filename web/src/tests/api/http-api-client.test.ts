import { describe, expect, it, vi } from 'vitest';
import { stringify as stringifyLosslessJSON } from 'lossless-json';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient', () => {
  it('uses same-origin session endpoints and treats unauthorized as anonymous', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: 'unauthorized',
          detail: 'A management session is required.',
        }),
        {
          status: 401,
          headers: { 'Content-Type': 'application/problem+json' },
        },
      ),
    );
    const client = createHttpApiClient({ baseUrl: '/api/v1/', fetcher });

    await expect(client.getSession()).resolves.toBeNull();
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/auth/session',
      expect.objectContaining({ credentials: 'same-origin', method: 'GET' }),
    );
  });

  it('preserves problem codes and recovery detail from the API', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: 'runtime_unavailable',
          detail: 'The runtime executor lease is not held.',
        }),
        {
          status: 503,
          headers: { 'Content-Type': 'application/problem+json' },
        },
      ),
    );
    const client = createHttpApiClient({ fetcher });

    await expect(client.getDashboardContext()).rejects.toMatchObject({
      code: 'runtime_unavailable',
      message: 'The runtime executor lease is not held.',
      status: 503,
    });
  });

  it('sends login JSON without dropping the common Accept header', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ displayName: 'Panel administrator' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    const client = createHttpApiClient({ fetcher });

    await client.login('secret-token');

    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/auth/session',
      expect.objectContaining({
        body: JSON.stringify({ token: 'secret-token' }),
        headers: {
          'Accept': 'application/json',
          'Content-Type': 'application/json',
        },
        method: 'POST',
      }),
    );
  });

  it('retains the session CSRF token for cookie-authenticated writes', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            displayName: 'Panel administrator',
            csrfToken: 'csrf-from-session',
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    const client = createHttpApiClient({ fetcher });

    await client.login('secret-token');
    await client.logout();

    expect(fetcher).toHaveBeenLastCalledWith(
      '/api/v1/auth/session',
      expect.objectContaining({
        method: 'DELETE',
        headers: {
          'Accept': 'application/json',
          'X-CSRF-Token': 'csrf-from-session',
        },
      }),
    );
  });

  it('sends entity edits with CSRF and an exact If-Match revision', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            displayName: 'Panel administrator',
            csrfToken: 'csrf-write',
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ revision: {}, no_change: false }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    const client = createHttpApiClient({ fetcher });

    await client.login('secret-token');
    await client.saveEntity(
      'nodes',
      { id: 'edge', kind: 'socks', enabled: true },
      { baseRevision: 'revision_42', existingID: 'edge' },
    );

    expect(fetcher).toHaveBeenLastCalledWith(
      '/api/v1/nodes/edge',
      expect.objectContaining({
        body: JSON.stringify({ id: 'edge', kind: 'socks', enabled: true }),
        method: 'PUT',
        headers: {
          'Accept': 'application/json',
          'Content-Type': 'application/json',
          'If-Match': '"revision_42"',
          'X-CSRF-Token': 'csrf-write',
        },
      }),
    );
  });

  it('derives entity data from document_json and preserves exact numbers on save', async () => {
    const documentJSON = '{"schema_version":1,"global":{},"nodes":[{"id":"edge","kind":"socks","enabled":true,"quota":9007199254740993,"huge":1e999}],"rules":[],"subscription":{}}';
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({
          revision: {
            id: 'revision_lossless',
            sequence: 1,
            schema_version: 1,
            document: {
              schema_version: 1,
              global: {},
              nodes: [{ id: 'edge', kind: 'socks', enabled: true, quota: 9007199254740992, huge: null }],
              rules: [],
              subscription: {},
            },
            document_json: documentJSON,
            sha256: 'a'.repeat(64),
            created_at: '2026-08-26T07:30:00Z',
          },
          entities: [{ id: 'edge', kind: 'socks', enabled: true, quota: 9007199254740992, huge: null }],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ revision: {}, no_change: false }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    const client = createHttpApiClient({ fetcher });

    const listed = await client.listEntities('nodes');
    expect(stringifyLosslessJSON(listed.entities[0])).toBe(
      '{"id":"edge","kind":"socks","enabled":true,"quota":9007199254740993,"huge":1e999}',
    );

    await client.saveEntity('nodes', listed.entities[0], {
      baseRevision: 'revision_lossless',
      existingID: 'edge',
    });

    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      '/api/v1/nodes/edge',
      expect.objectContaining({
        body: '{"id":"edge","kind":"socks","enabled":true,"quota":9007199254740993,"huge":1e999}',
        headers: expect.objectContaining({ 'If-Match': '"revision_lossless"' }),
        method: 'PUT',
      }),
    );
  });

  it('replaces one complete canonical document from lossless JSON text with exact If-Match evidence', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ displayName: 'Panel administrator', csrfToken: 'csrf-canonical' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ revision: {}, no_change: false }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    const client = createHttpApiClient({ fetcher });
    const documentJSON = '{"schema_version":1,"global":{"large":9007199254740993},"nodes":[],"rules":[],"subscription":{}}';

    await client.login('secret-token');
    await client.replaceCanonical(documentJSON, 'revision_42');

    expect(fetcher).toHaveBeenLastCalledWith(
      '/api/v1/config/canonical',
      expect.objectContaining({
        body: documentJSON,
        method: 'PUT',
        headers: {
          'Accept': 'application/json',
          'Content-Type': 'application/json',
          'If-Match': '"revision_42"',
          'X-CSRF-Token': 'csrf-canonical',
        },
      }),
    );
  });

  it('sends lossless canonical pointer changes with exact If-Match evidence', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ displayName: 'Panel administrator', csrfToken: 'csrf-patch' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ revision: {}, no_change: false }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    const client = createHttpApiClient({ fetcher });
    const changes = [
      { op: 'set' as const, path: '/global/large', value_json: '9007199254740993' },
      { op: 'set' as const, path: '/global/huge', value_json: '1e999' },
      { op: 'unset' as const, path: '/global/obsolete' },
    ];

    await client.login('secret-token');
    await client.patchCanonical(changes, 'revision_42');

    expect(fetcher).toHaveBeenLastCalledWith(
      '/api/v1/config/canonical',
      expect.objectContaining({
        body: JSON.stringify({ changes }),
        method: 'PATCH',
        headers: {
          'Accept': 'application/json',
          'Content-Type': 'application/json',
          'If-Match': '"revision_42"',
          'X-CSRF-Token': 'csrf-patch',
        },
      }),
    );
  });

  it('queries capability with an explicit version and never an implicit latest', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          resolution: { exact_version: '1.12.7', source: 'explicit' },
          support_level: 'manual_json',
          pinned: false,
          quarantined: false,
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await client.getCoreCapability('1.12.7');

    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/core/capability?core_version=1.12.7',
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('preserves every core artifact filter and keyset cursor', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ items: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await client.listCoreArtifacts({
      architecture: 'amd64',
      beforeID: 'core_older',
      beforeTime: '2026-08-26T07:00:00.000000001Z',
      exactVersion: '1.13.19',
      limit: 200,
      sourceKind: 'official',
      variant: 'plain',
      verificationState: 'verified',
    });

    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/core/artifacts?architecture=amd64&before_id=core_older&before_time=2026-08-26T07%3A00%3A00.000000001Z&exact_version=1.13.19&limit=200&source_kind=official&variant=plain&verification_state=verified',
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('uses explicit monotonic artifact verification endpoints', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ id: 'core/one', verification_state: 'revoked' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await client.quarantineCoreArtifact('core/one');
    await client.revokeCoreArtifact('core/one');

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      '/panel/api/v1/core/artifacts/core%2Fone/quarantine',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      '/panel/api/v1/core/artifacts/core%2Fone/revoke',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('keeps manual JSON in the raw body and sends exact version, artifact and revision evidence', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ displayName: 'Panel administrator', csrfToken: 'csrf-manual' }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ reverse: {} }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ artifact: {}, revision: {}, task: {} }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    const client = createHttpApiClient({ fetcher });
    const raw = '{\n // exact comment\n "log": {"level":"warn"}\n}\n';

    await client.login('secret-token');
    await client.previewManualReplacement({
      baseRevision: 'revision_42',
      coreVersion: '1.13.19',
      coreArtifactID: 'core_1',
      raw,
      allowCompatible: true,
    });
    expect(fetcher).toHaveBeenLastCalledWith(
      '/api/v1/config/manual/preview?core_version=1.13.19&core_artifact_id=core_1&allow_compatible=true',
      expect.objectContaining({
        body: raw,
        method: 'POST',
        headers: {
          'Accept': 'application/json',
          'Content-Type': 'application/jsonc',
          'If-Match': '"revision_42"',
          'X-CSRF-Token': 'csrf-manual',
        },
      }),
    );
    await client.replaceManualArtifact({
      baseRevision: 'revision_42',
      coreVersion: '1.13.19',
      coreArtifactID: 'core_1',
      raw,
    });

    expect(fetcher).toHaveBeenLastCalledWith(
      '/api/v1/config/manual?core_version=1.13.19&core_artifact_id=core_1',
      expect.objectContaining({
        body: raw,
        method: 'PUT',
        headers: {
          'Accept': 'application/json',
          'Content-Type': 'application/jsonc',
          'If-Match': '"revision_42"',
          'X-CSRF-Token': 'csrf-manual',
        },
      }),
    );
  });

  it('uses updated_at as subscription CAS evidence and never puts plaintext in metadata URLs', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ displayName: 'Panel administrator', csrfToken: 'csrf-subscription' }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: 'channel_1' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ metadata: {}, token: 'once-only' }), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    const client = createHttpApiClient({ fetcher });

    await client.login('secret-token');
    await client.updateSubscriptionChannel(
      'channel/1',
      { name: 'Primary', format: 'sing-box', config: {}, enabled: true },
      '2026-08-26T07:05:00.000000001Z',
    );
    await client.createSubscriptionToken({});

    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      '/api/v1/subscription/channels/channel%2F1',
      expect.objectContaining({
        method: 'PUT',
        headers: expect.objectContaining({
          'If-Match': '"2026-08-26T07:05:00.000000001Z"',
          'X-CSRF-Token': 'csrf-subscription',
        }),
      }),
    );
    expect(fetcher.mock.calls[2]?.[0]).toBe('/api/v1/subscription/tokens');
    expect(String(fetcher.mock.calls[2]?.[0])).not.toContain('once-only');
  });

  it('centralizes observability filters on the stable read-only endpoints', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ items: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await client.listLogs({
      source: 'core',
      level: 'warn',
      since: '2026-08-26T00:00:00Z',
      limit: 20,
      afterTime: '2026-08-26T07:00:00Z',
      afterID: 'log_1',
    });

    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/logs?source=core&level=warn&since=2026-08-26T00%3A00%3A00Z&limit=20&after_time=2026-08-26T07%3A00%3A00Z&after_id=log_1',
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('uses stable explicit durable-log deletion endpoints', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ deleted: 1 }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await client.clearLogs({ source: 'security', before: '2026-08-26T08:00:00Z' });
    await client.deleteLog('log/one');

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      '/panel/api/v1/logs?before=2026-08-26T08%3A00%3A00Z&source=security',
      expect.objectContaining({ method: 'DELETE' }),
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      '/panel/api/v1/logs/log%2Fone',
      expect.objectContaining({ method: 'DELETE' }),
    );
  });
});
