import { describe, expect, it, vi } from 'vitest';

import { createHttpApiClient } from '@/api/http-api-client';

describe('panel configuration backup HTTP client', () => {
  it('creates, stages, plans and applies backups by category with CSRF, and names a backup in its download address', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({ csrfToken: 'backup-csrf' })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ name: 'b.tar.gz' }), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: '7' })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ signs_out: false })))
      .mockResolvedValueOnce(new Response(JSON.stringify({ restarting: true }), { status: 202 }));
    const client = createHttpApiClient({ baseUrl: '/control/api/v1', fetcher });
    await client.getSession();
    await client.createPanelBackup({ categories: ['rule_sets', 'keys'] });
    expect(fetcher).toHaveBeenLastCalledWith('/control/api/v1/panel/backups', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ categories: ['rule_sets', 'keys'] }), headers: expect.objectContaining({ 'X-CSRF-Token': 'backup-csrf' }),
    }));
    await client.stagePanelBackup('sing-box-panel-20261127-030000-scheduled.tar.gz', {});
    expect(fetcher).toHaveBeenLastCalledWith('/control/api/v1/panel/backups/sing-box-panel-20261127-030000-scheduled.tar.gz/restore', expect.objectContaining({ method: 'POST' }));
    await client.planPanelRestore('7', { categories: ['keys'] });
    expect(fetcher).toHaveBeenLastCalledWith('/control/api/v1/panel/restores/7/plan', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ categories: ['keys'] }), headers: expect.objectContaining({ 'X-CSRF-Token': 'backup-csrf' }),
    }));
    await client.applyPanelRestore('7', { take_service: false, categories: ['keys'] });
    expect(fetcher).toHaveBeenLastCalledWith('/control/api/v1/panel/restores/7/apply', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ take_service: false, categories: ['keys'] }),
    }));
    expect(client.panelBackupDownloadUrl('a b.tar.gz')).toBe('/control/api/v1/panel/backups/a%20b.tar.gz');
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
