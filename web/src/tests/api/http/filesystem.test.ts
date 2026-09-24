import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('filesystem HTTP API', () => {
  it('encodes server paths, preserves base paths and cancels read-only requests', async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => new Response('{}'));
    const client = createHttpApiClient({ baseUrl: '/panel/api/v1', fetcher });
    const signal = new AbortController().signal;
    const path = '/etc/中文 #file?+.pem';
    await client.listFilesystemEntries({ path, search: '证书', show_hidden: true, offset: 10, limit: 10 }, signal);
    await client.resolveFilesystemPath({ path, mode: 'output-file' }, signal);
    const url = new URL(String(fetcher.mock.calls[0][0]), 'http://panel');
    expect(url.pathname).toBe('/panel/api/v1/filesystem/entries');
    expect(Object.fromEntries(url.searchParams)).toEqual({ path, search: '证书', show_hidden: 'true', offset: '10', limit: '10' });
    expect(new URL(String(fetcher.mock.calls[1][0]), 'http://panel').searchParams.get('path')).toBe(path);
    for (const [, init] of fetcher.mock.calls) {
      expect(init).toMatchObject({ method: 'GET', credentials: 'same-origin', signal });
      expect(init?.body).toBeUndefined();
    }
  });

  it('preserves filesystem problem codes', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ code: 'filesystem_forbidden', detail: 'Denied' }), { status: 403 }));
    await expect(createHttpApiClient({ fetcher }).listFilesystemEntries()).rejects.toMatchObject({ code: 'filesystem_forbidden', status: 403 });
  });
});
