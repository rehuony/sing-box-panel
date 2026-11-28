import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient subscription domain', () => {
  it('saves each list\'s order as its IDs at the list\'s order path', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => new Response(null, { status: 204 }));
    const client = createHttpApiClient({ fetcher });
    await client.setSubscriptionSourceOrder(['source-b', 'source-a']);
    await client.setSubscriptionChannelOrder(['channel-b', 'channel-a']);
    await client.setSubscriptionTokenOrder(['token-b', 'token-a']);
    expect(fetcher.mock.calls.map(([url, init]) => [url, init?.method, init?.body])).toEqual([
      ['/api/v1/subscription/source-order', 'PUT', '{"ids":["source-b","source-a"]}'],
      ['/api/v1/subscription/channel-order', 'PUT', '{"ids":["channel-b","channel-a"]}'],
      ['/api/v1/subscription/token-order', 'PUT', '{"ids":["token-b","token-a"]}'],
    ]);
  });

  it('decodes UTF-8 preview bytes and sends isolated unsaved template input', async () => {
    const text = '{"outbounds":[{"tag":"香港"}]}';
    const encoded = btoa(String.fromCharCode(...new TextEncoder().encode(text)));
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response(JSON.stringify({ result: { content: encoded } }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    const client = createHttpApiClient({ fetcher });
    const draft = { format: 'sing-box' as const, config: { groups: [], routes: [], default_exit: { kind: 'direct' as const } } };
    const result = await client.previewSubscriptionChannel('channel/one', '', undefined, draft);
    expect(result.result.content).toBe(text);
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/subscription/channels/channel%2Fone/preview',
      expect.objectContaining({ body: JSON.stringify({ user_id: '', draft }) }),
    );
  });

  it('reads token metadata by encoded identifier', async () => {
    const token = { id: 'token/1', label: 'phone' };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(
      new Response(JSON.stringify(token), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await expect(client.getSubscriptionToken('token/1')).resolves.toEqual(token);
    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/subscription/tokens/token%2F1',
      expect.objectContaining({ method: 'GET' }),
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

    await client.login({ email: 'admin@example.com', password: 'test-password-123' });
    await client.updateSubscriptionChannel(
      'channel/1',
      {
        name: 'Primary',
        format: 'sing-box',
        config: { groups: [], routes: [], default_exit: { kind: 'direct' } },
        enabled: true,
      },
      '2026-08-26T07:05:00.000000001Z',
    );
    await client.createSubscriptionToken({ userID: 'user_1', label: 'phone' });

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

  it('saves the whole pool order and renames a node at the version it was read', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => new Response('{}', { status: 200 }));
    const client = createHttpApiClient({ fetcher });
    await client.setSubscriptionNodeOrder(['sn_b', 'sn_a']);
    await client.updateSubscriptionNode('sn_a', { display_name: 'tokyo' }, '2026-11-19T08:00:00.000000001Z');
    expect(fetcher).toHaveBeenNthCalledWith(1, '/api/v1/subscription/node-order', expect.objectContaining({
      method: 'PUT', body: JSON.stringify({ ids: ['sn_b', 'sn_a'] }),
    }));
    expect(fetcher).toHaveBeenNthCalledWith(2, '/api/v1/subscription/nodes/sn_a', expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ display_name: 'tokyo' }),
      headers: expect.objectContaining({ 'If-Match': '"2026-11-19T08:00:00.000000001Z"' }),
    }));
  });
});
