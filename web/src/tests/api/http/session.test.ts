import { beforeEach, describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

const fingerprint = vi.hoisted(() => vi.fn<() => Promise<string | undefined>>());
vi.mock('@/api/http/login-fingerprint', () => ({ loginFingerprint: fingerprint }));
beforeEach(() => {
  fingerprint.mockResolvedValue(undefined);
});

describe('createHttpApiClient session domain', () => {
  it('reads the complete control-plane system status', async () => {
    const status = {
      panel_version: '0.1.0',
      canonical_revision: 42,
      applied_bundle_id: 'bundle_18',
      running: true,
      running_version: '1.13.19',
      running_artifact: 'core_1',
      configuration_state: 'sing-box-1.13.19@1',
    };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify(status), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });

    await expect(client.getSystemStatus()).resolves.toEqual(status);
    expect(fetcher).toHaveBeenCalledWith(
      '/panel/api/v1/system/status',
      expect.objectContaining({ credentials: 'same-origin', method: 'GET' }),
    );
  });

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
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () =>
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

    await client.login({ email: 'admin@example.com', password: 'test-password-123' });

    expect(fetcher).toHaveBeenCalledWith(
      '/api/v1/auth/session',
      expect.objectContaining({
        body: JSON.stringify({ email: 'admin@example.com', password: 'test-password-123' }),
        headers: {
          'Accept': 'application/json',
          'Content-Type': 'application/json',
        },
        method: 'POST',
      }),
    );
  });

  it('sends the fingerprint only on login and never retries a rate-limited login', async () => {
    fingerprint.mockResolvedValue('a'.repeat(32));
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(new Response(JSON.stringify({ code: 'login_rate_limited' }), {
      status: 429,
      headers: { 'Retry-After': '60', 'Content-Type': 'application/problem+json' },
    })).mockResolvedValueOnce(new Response(JSON.stringify({ displayName: 'Administrator' })));
    const client = createHttpApiClient({ fetcher });
    await expect(client.login({ email: 'admin@example.com', password: 'test-password-123' })).rejects.toMatchObject({ status: 429, code: 'login_rate_limited' });
    expect(fetcher).toHaveBeenCalledOnce();
    expect(fetcher).toHaveBeenCalledWith('/api/v1/auth/session', expect.objectContaining({
      headers: expect.objectContaining({ 'X-Client-Fingerprint': 'a'.repeat(32) }),
      body: JSON.stringify({ email: 'admin@example.com', password: 'test-password-123' }),
    }));
    await client.getSession();
    expect(fetcher).toHaveBeenLastCalledWith('/api/v1/auth/session', expect.objectContaining({ headers: { Accept: 'application/json' } }));
  });

  it('does not send credentials when cancelled during fingerprint collection', async () => {
    const controller = new AbortController();
    fingerprint.mockImplementation(async () => {
      controller.abort();
      return undefined;
    });
    const fetcher = vi.fn<typeof fetch>();
    const client = createHttpApiClient({ fetcher });
    await expect(client.login({ email: 'admin@example.com', password: 'test-password-123' }, controller.signal)).rejects.toMatchObject({ name: 'AbortError' });
    expect(fetcher).not.toHaveBeenCalled();
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

    await client.login({ email: 'admin@example.com', password: 'test-password-123' });
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

  it('invalidates the local session and clears CSRF after any unauthorized response', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ displayName: 'Panel administrator', csrfToken: 'csrf-active' }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ))
      .mockResolvedValueOnce(new Response(
        JSON.stringify({ code: 'unauthorized', detail: 'Session expired.' }),
        { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
      ))
      .mockResolvedValueOnce(new Response(JSON.stringify({}), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }));
    const client = createHttpApiClient({ fetcher });
    const invalidated = vi.fn();
    client.subscribeSessionInvalidated(invalidated);

    await client.login({ email: 'admin@example.com', password: 'test-password-123' });
    await expect(client.getDashboardContext()).rejects.toMatchObject({ status: 401 });
    await client.stopRuntime();

    expect(invalidated).toHaveBeenCalledOnce();
    expect(fetcher).toHaveBeenLastCalledWith(
      '/api/v1/core/stop',
      expect.objectContaining({
        headers: { Accept: 'application/json' },
        method: 'POST',
      }),
    );
  });

  it('treats an unauthorized logout response as locally signed out', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(
      JSON.stringify({ code: 'unauthorized', detail: 'Session already expired.' }),
      { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
    ));
    const client = createHttpApiClient({ fetcher });
    const invalidated = vi.fn();
    client.subscribeSessionInvalidated(invalidated);

    await expect(client.logout()).resolves.toBeUndefined();
    expect(invalidated).toHaveBeenCalledOnce();
  });
  it.each([503, 'network'])('retains restored CSRF and identity during %s failures without replaying writes', async failure => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({ email: 'admin@example.com', displayName: 'Administrator', csrfToken: 'restored-csrf', expiresAt: '2099-01-01T00:00:00Z' })))
      .mockImplementationOnce(async () => {
        if (failure === 'network') throw new TypeError('offline');
        return new Response(JSON.stringify({ code: 'authentication_unavailable' }), { status: 503 });
      })
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    const client = createHttpApiClient({ fetcher });
    const invalidated = vi.fn();
    client.subscribeSessionInvalidated(invalidated);
    await client.getSession();
    await expect(client.stopRuntime()).rejects.toBeInstanceOf(Error);
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(invalidated).not.toHaveBeenCalled();
    await client.logout();
    expect(fetcher).toHaveBeenLastCalledWith('/api/v1/auth/session', expect.objectContaining({
      headers: expect.objectContaining({ 'X-CSRF-Token': 'restored-csrf' }),
    }));
  });
});
