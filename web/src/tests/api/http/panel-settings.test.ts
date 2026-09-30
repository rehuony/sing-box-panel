import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('panel configuration backup HTTP client', () => {
  it('exports saved settings and restores with both revision guards and CSRF', async () => {
    const backup = {
      format: 'sing-box-panel-backup' as const, version: 2 as const, exported_at: '2026-09-23T01:00:00Z',
      panel_settings: {}, sing_box_configuration: 'exact raw content',
    };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({ csrfToken: 'restore-csrf' })))
      .mockResolvedValueOnce(new Response(JSON.stringify(backup)))
      .mockResolvedValueOnce(new Response(JSON.stringify({ reauthentication_required: true })));
    const client = createHttpApiClient({ baseUrl: '/control/api/v1', fetcher });
    await client.getSession();
    expect(await client.exportPanelBackup()).toEqual(backup);
    expect(fetcher).toHaveBeenLastCalledWith('/control/api/v1/panel/backup', expect.objectContaining({ method: 'GET' }));
    const request = { backup, settings_revision: 14, configuration_revision: 7 };
    await client.restorePanelBackup(request);
    expect(fetcher).toHaveBeenLastCalledWith('/control/api/v1/panel/restore', expect.objectContaining({
      method: 'POST', credentials: 'same-origin', body: JSON.stringify(request),
      headers: expect.objectContaining({ 'X-CSRF-Token': 'restore-csrf' }),
    }));
  });
  it('invalidates the session only after an actual credential change', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 'panel_settings_invalid' }), { status: 422 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ settings: {}, reauthentication_required: false })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ settings: {}, reauthentication_required: true })));
    const client = createHttpApiClient({ fetcher });
    const invalidated = vi.fn();
    client.subscribeSessionInvalidated(invalidated);
    const input = { revision: 1, preferences: {} as Parameters<typeof client.savePanelSettings>[0]['preferences'], credentials: { email: 'new@example.com' } };
    await expect(client.savePanelSettings(input)).rejects.toMatchObject({ status: 422, code: 'panel_settings_invalid' });
    expect(invalidated).not.toHaveBeenCalled();
    await client.savePanelSettings(input);
    expect(invalidated).not.toHaveBeenCalled();
    await client.savePanelSettings(input);
    expect(invalidated).toHaveBeenCalledOnce();
  });
});
