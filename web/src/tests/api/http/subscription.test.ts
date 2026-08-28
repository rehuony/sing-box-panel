import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient subscription domain', () => {
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
      { name: 'Primary', format: 'sing-box', public_host: 'proxy.example', config: {}, enabled: true },
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
});
