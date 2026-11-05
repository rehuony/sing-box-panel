import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';
import { testDashboardSnapshot, testMetrics, testRuntimeStatus } from '@/tests/api/mock-api-client';

describe('createHttpApiClient observability domain', () => {
  it('decodes live metrics and rejects an empty successful stream', async () => {
    const snapshot = { metrics: testMetrics, runtime: testRuntimeStatus };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(`event: metrics\ndata: ${JSON.stringify(snapshot)}\n\n`, {
        headers: { 'Content-Type': 'text/event-stream' },
      }))
      .mockResolvedValueOnce(new Response(': keepalive\n\n', {
        headers: { 'Content-Type': 'text/event-stream' },
      }));
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    const snapshots = [];
    for await (const event of client.streamMetrics()) snapshots.push(event);
    expect(snapshots).toEqual([snapshot]);
    await expect(client.streamMetrics()[Symbol.asyncIterator]().next()).rejects.toMatchObject({
      code: 'stream_invalid', status: 200,
    });
  });

  it('surfaces the initial metrics collection failure', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({
      code: 'metrics_snapshot_unavailable', detail: 'The metrics snapshot could not be collected.',
    }), { status: 500, headers: { 'Content-Type': 'application/problem+json' } }));
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    await expect(client.streamMetrics()[Symbol.asyncIterator]().next()).rejects.toMatchObject({
      code: 'metrics_snapshot_unavailable', status: 500,
    });
  });

  it('clears only the selected capture with CSRF protection and surfaces failures', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({ csrfToken: 'log-csrf-token' })))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 'core_log_clear_failed' }), {
        status: 503, headers: { 'Content-Type': 'application/problem+json' },
      }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher }).forNode('node_a');
    await client.getSession();
    await client.clearCoreLog('2026-09-23-000.log');
    expect(fetcher).toHaveBeenLastCalledWith('/panel/api/v1/nodes/node_a/core/logs/content?file=2026-09-23-000.log', expect.objectContaining({
      method: 'DELETE', credentials: 'same-origin', headers: expect.objectContaining({ 'X-CSRF-Token': 'log-csrf-token' }),
    }));
    await expect(client.clearCoreLog('2026-09-23-000.log')).rejects.toMatchObject({ status: 503, code: 'core_log_clear_failed' });
  });
  it('deletes a managed log by encoded name with the session CSRF token', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({ csrfToken: 'log-csrf-token' })))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher }).forNode('node_a');
    await client.getSession();
    await client.deleteCoreLogFile('2026-09-18-000.log');
    expect(fetcher).toHaveBeenLastCalledWith('/panel/api/v1/nodes/node_a/core/logs/files?file=2026-09-18-000.log', expect.objectContaining({
      method: 'DELETE', credentials: 'same-origin', headers: expect.objectContaining({ 'X-CSRF-Token': 'log-csrf-token' }),
    }));
  });
  it('requests filtered numbered panel-log pages', async () => {
    const page = { items: [], total: 27 };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify(page)));
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    await expect(client.listPanelLogs({ offset: 20, limit: 10, search: 'refresh', level: 'info' })).resolves.toEqual(page);
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/logs/panel?offset=20&limit=10&search=refresh&level=info', expect.objectContaining({ method: 'GET' }),
    );
  });

  it('serializes translated search codes as one comma-separated query parameter', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
      new Response(JSON.stringify({ items: [], total: 0 })),
    );
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    await client.listPanelLogs({ search: '核心启动', searchCodes: ['runtime.start.completed', 'start_succeeded'] });
    const url = new URL(String(fetcher.mock.calls[0]![0]), 'https://panel.test');
    expect(url.searchParams.get('search')).toBe('核心启动');
    expect(url.searchParams.getAll('search_codes')).toEqual(['runtime.start.completed,start_succeeded']);
    await client.listPanelLogs({ searchCodes: [] });
    expect(String(fetcher.mock.calls[1]![0])).not.toContain('search_codes');
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
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    const invalidated = vi.fn();
    client.subscribeSessionInvalidated(invalidated);
    const iterator = client.streamDashboard()[Symbol.asyncIterator]();

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
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher }).forNode('node_a');
    const snapshots = [];

    for await (const snapshot of client.streamDashboard()) snapshots.push(snapshot);

    expect(snapshots).toEqual([testDashboardSnapshot]);
    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/nodes/node_a/dashboard/stream',
      expect.objectContaining({
        credentials: 'same-origin',
        headers: { Accept: 'text/event-stream' },
        method: 'GET',
      }),
    );
  });

  it('surfaces a failed initial dashboard snapshot as an API error', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({
      code: 'dashboard_snapshot_unavailable',
      detail: 'The dashboard snapshot could not be collected.',
    }), {
      status: 500,
      headers: { 'Content-Type': 'application/problem+json' },
    }));
    const client = createHttpApiClient({ fetcher }).forNode('node_a');

    await expect(client.streamDashboard()[Symbol.asyncIterator]().next()).rejects.toMatchObject({
      code: 'dashboard_snapshot_unavailable',
      status: 500,
    });
  });

  it('rejects a dashboard stream that closes without a snapshot', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(': keepalive\n\n', {
      headers: { 'Content-Type': 'text/event-stream' },
    }));
    const client = createHttpApiClient({ fetcher }).forNode('node_a');

    await expect(client.streamDashboard()[Symbol.asyncIterator]().next()).rejects.toMatchObject({
      code: 'stream_invalid',
      status: 200,
    });
  });
});

describe('native output and panel activity', () => {
  it('passes selected file and resume offset and decodes native SSE', async () => {
    const chunk = { file: '2026-09-19-000.log', text: 'INFO 节点\n', generation: 'generation-2', reset: true, next_offset: 13, size: 13 };
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response(`event: output\ndata: ${JSON.stringify(chunk)}\n\n`, {
          headers: { 'Content-Type': 'text/event-stream' },
        }),
      );
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    const events = [];
    for await (const event of client.streamCoreLog(chunk.file, 5, 'generation-1')) events.push(event);
    expect(events).toEqual([chunk]);
    expect(fetcher.mock.calls[0][0]).toContain('file=2026-09-19-000.log&offset=5&generation=generation-1');
  });
  it('encodes combined panel filters', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(
        async () => new Response('{}', { headers: { 'Content-Type': 'application/json' } }),
      );
    const client = createHttpApiClient({ fetcher }).forNode('node_a');
    await client.listPanelLogs({
      search: 'a&b',
      beforeID: 'log:x',
      beforeTime: '2026-09-19T00:00:00Z',
      limit: 5,
    });
    expect(fetcher.mock.calls[0][0]).toContain('search=a%26b');
  });
});
