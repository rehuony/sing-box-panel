import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient subscription domain', () => {
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
    const draft = { format: 'sing-box' as const, config: {} };
    const result = await client.previewSubscriptionChannel('channel/one', '', undefined, draft);
    expect(result.result.content).toBe(text);
    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/subscription/channels/channel%2Fone/preview',
      expect.objectContaining({ body: JSON.stringify({ user_id: '', draft }) }),
    );
  });

  it('reads token metadata and immutable source-version details by encoded identifiers', async () => {
    const token = { id: 'token/1', label: 'phone' };
    const version = { id: 'version/1', source_id: 'source/1' };
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(token), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(version), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      );
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await expect(client.getSubscriptionToken('token/1')).resolves.toEqual(token);
    await expect(client.getSubscriptionSourceVersion('source/1', 'version/1')).resolves.toEqual(
      version,
    );

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      '/panel/api/v1/subscription/tokens/token%2F1',
      expect.objectContaining({ method: 'GET' }),
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      '/panel/api/v1/subscription/sources/source%2F1/versions/version%2F1',
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

    await client.login('secret-token');
    await client.updateSubscriptionChannel(
      'channel/1',
      {
        name: 'Primary',
        format: 'sing-box',
        public_host: 'proxy.example',
        config: {},
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
});
