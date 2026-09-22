import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';
import { testDashboardSnapshot } from '@/tests/api/mock-api-client';

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
      code: 'runtime.health_failed',
      since: '2026-08-26T00:00:00Z',
      until: '2026-08-27T00:00:00Z',
      limit: 20,
      afterTime: '2026-08-26T07:00:00Z',
      afterID: 'log_1',
    });

    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/logs?source=core&level=warn&code=runtime.health_failed&since=2026-08-26T00%3A00%3A00Z&until=2026-08-27T00%3A00%3A00Z&limit=20&after_time=2026-08-26T07%3A00%3A00Z&after_id=log_1',
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('decodes chunked durable-log SSE events and preserves the resume cursor', async () => {
    const entry = {
      id: 'log_1',
      time: '2026-08-26T07:00:00Z',
      source: 'core',
      level: 'info',
      code: 'runtime.ready',
      message: 'Runtime ready.',
      metadata: {},
    } as const;
    const encoder = new TextEncoder();
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode(': keepalive\n\nid: 2026-08-26T07:00:00Z|log_1\nevent: log\nda'),
        );
        controller.enqueue(encoder.encode(`ta: ${JSON.stringify(entry)}\n\n`));
        controller.close();
      },
    });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(body, {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });
    const events = [];

    for await (const event of client.streamLogs({
      source: 'core',
      code: 'runtime.ready',
      limit: 10,
      lastEventID: '2026-08-26T06:59:00Z|log_0',
    })) {
      events.push(event);
    }

    expect(events).toEqual([
      {
        id: '2026-08-26T07:00:00Z|log_1',
        entry,
      },
    ]);
    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/logs/stream?source=core&code=runtime.ready&limit=10',
      expect.objectContaining({
        credentials: 'same-origin',
        headers: {
          'Accept': 'text/event-stream',
          'Last-Event-ID': '2026-08-26T06:59:00Z|log_0',
        },
        method: 'GET',
      }),
    );
  });

  it('invalidates the local session when an SSE connection is unauthorized', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: 'unauthorized',
          detail: 'Session expired.',
        }),
        {
          status: 401,
          headers: { 'Content-Type': 'application/problem+json' },
        },
      ),
    );
    const client = createHttpApiClient({ fetcher });
    const invalidated = vi.fn();
    client.subscribeSessionInvalidated(invalidated);
    const iterator = client.streamLogs()[Symbol.asyncIterator]();

    await expect(iterator.next()).rejects.toMatchObject({ code: 'unauthorized', status: 401 });
    expect(invalidated).toHaveBeenCalledOnce();
  });

  it('decodes dashboard snapshots from the authenticated SSE endpoint', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(`event: dashboard\ndata: ${JSON.stringify(testDashboardSnapshot)}\n\n`, {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });
    const snapshots = [];

    for await (const snapshot of client.streamDashboard()) snapshots.push(snapshot);

    expect(snapshots).toEqual([testDashboardSnapshot]);
    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/dashboard/stream',
      expect.objectContaining({
        credentials: 'same-origin',
        headers: { Accept: 'text/event-stream' },
        method: 'GET',
      }),
    );
  });

  it('uses stable explicit durable-log deletion endpoints', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(
      async () =>
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

  it('preserves nullable metric-history evidence and paired traffic cursors', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(
      async () =>
        new Response(
          JSON.stringify({
            from: '2026-08-26T00:00:00Z',
            to: '2026-08-26T01:00:00Z',
            bucket_seconds: 300,
            buckets: [
              {
                from: '2026-08-26T00:00:00Z',
                to: '2026-08-26T00:05:00Z',
                upload_bytes: null,
                download_bytes: null,
                memory_bytes_avg: null,
                memory_bytes_peak: null,
                active_connections_avg: null,
                active_connections_peak: null,
                sample_count: 0,
                coverage: 'missing',
              },
            ],
            items: [],
          }),
          {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          },
        ),
    );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    const history = await client.getMetricsHistory({
      from: '2026-08-26T00:00:00Z',
      to: '2026-08-26T01:00:00Z',
      bucketSeconds: 300,
      activationBundleID: 'bundle_18',
    });
    await client.listTrafficPeriods({
      activationBundleID: 'bundle_18',
      from: '2026-08-26T00:00:00Z',
      to: '2026-08-27T00:00:00Z',
      beforeTime: '2026-08-26T12:00:00Z',
      beforeID: 'period_42',
      limit: 20,
    });

    expect(history.buckets[0]).toMatchObject({
      upload_bytes: null,
      download_bytes: null,
      coverage: 'missing',
    });
    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      '/panel/api/v1/metrics/history?from=2026-08-26T00%3A00%3A00Z&to=2026-08-26T01%3A00%3A00Z&bucket_seconds=300&activation_bundle_id=bundle_18',
      expect.objectContaining({ method: 'GET' }),
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      '/panel/api/v1/traffic/periods?activation_bundle_id=bundle_18&from=2026-08-26T00%3A00%3A00Z&to=2026-08-27T00%3A00%3A00Z&before_time=2026-08-26T12%3A00%3A00Z&before_id=period_42&limit=20',
      expect.objectContaining({ method: 'GET' }),
    );
  });
});

describe('native output and panel activity', () => {
  it('passes selected file and resume offset and decodes native SSE', async () => {
    const chunk = { file: '2026-09-19-000.log', text: 'INFO 节点\n', next_offset: 13, size: 13 };
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response(`event: output\ndata: ${JSON.stringify(chunk)}\n\n`, {
          headers: { 'Content-Type': 'text/event-stream' },
        }),
      );
    const client = createHttpApiClient({ fetcher });
    const events = [];
    for await (const event of client.streamCoreLog(chunk.file, 5)) events.push(event);
    expect(events).toEqual([chunk]);
    expect(fetcher.mock.calls[0][0]).toContain('file=2026-09-19-000.log&offset=5');
  });
  it('encodes combined panel filters', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(
        async () => new Response('{}', { headers: { 'Content-Type': 'application/json' } }),
      );
    const client = createHttpApiClient({ fetcher });
    await client.listPanelLogs({
      search: 'a&b',
      beforeID: 'log:x',
      beforeTime: '2026-09-19T00:00:00Z',
      limit: 5,
    });
    expect(fetcher.mock.calls[0][0]).toContain('search=a%26b');
  });
});
