import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient observability domain', () => {
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
