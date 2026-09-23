import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('panel configuration backup HTTP client', () => {
  it('exports saved settings and restores with both revision guards and CSRF', async () => {
    const backup = {
      format: 'sing-box-panel-backup' as const, version: 1 as const, exported_at: '2026-09-23T01:00:00Z',
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
});
