import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient canonical domain', () => {
  it('forwards the revision sequence cursor and requested page size', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ items: [] }), {
      status: 200, headers: { 'Content-Type': 'application/json' },
    }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await client.listRevisions({ beforeSequence: 41, limit: 24 });

    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/config/revisions?before_sequence=41&limit=24',
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('uses the one global canonical document and immutable revision routes', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ id: 'revision_2', revision: { id: 'revision_2' } }), {
        status: 200, headers: { 'Content-Type': 'application/json' },
      }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await client.replaceCanonical('{"schema_version":2,"configuration":{}}', 'revision_1');
    await client.restoreRevision('revision/old', 'revision_2');
    await client.diffRevisions('revision_1', 'revision_2');

    expect(fetcher).toHaveBeenNthCalledWith(1, '/panel/api/v1/config/canonical', expect.objectContaining({
      method: 'PUT', body: '{"schema_version":2,"configuration":{}}',
      headers: expect.objectContaining({ 'If-Match': '"revision_1"' }),
    }));
    expect(fetcher).toHaveBeenNthCalledWith(2, '/panel/api/v1/config/revisions/revision%2Fold/restore', expect.objectContaining({
      method: 'POST', headers: expect.objectContaining({ 'If-Match': '"revision_2"' }),
    }));
    expect(fetcher.mock.calls[2]?.[0]).toBe('/panel/api/v1/config/revisions/diff?from=revision_1&to=revision_2');
  });
});
