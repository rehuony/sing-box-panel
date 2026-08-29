import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('createHttpApiClient session domain', () => {
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

    await client.login('secret-token');
    await expect(client.getDashboardContext()).rejects.toMatchObject({ status: 401 });
    await client.cancelTask('task_1');

    expect(invalidated).toHaveBeenCalledOnce();
    expect(fetcher).toHaveBeenLastCalledWith(
      '/api/v1/tasks/task_1/cancel',
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
});
