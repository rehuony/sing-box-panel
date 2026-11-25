import { toast } from '@shared/components/ui/toast-manager';
import { afterEach, describe, expect, it, vi } from 'vitest';

import i18n from '@/i18n';
import { deferred } from '@/tests/deferred';
import { createQueryClient } from '@/api/query-client';
import { usePanelRestart } from '@/stores/panel-restart.store';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { useCommandSession } from '@/stores/command-session.store';
import { RESTART_TIMEOUT_MS, restartPanel, runReboot } from '@/components/status-line/run-restart';

const t = i18n.getFixedT('en');
const cache = createQueryClient();
const status = (started_at: string) => ({ panel_version: 'v1.0.0', platform: { arch: 'amd64', os: 'linux' }, started_at });

afterEach(() => {
  useCommandSession.getState().reset();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('panel restart', () => {
  it('prints the command line\'s reboot until the panel reports a later start, refusing a second one meanwhile', async () => {
    vi.useFakeTimers();
    const back = deferred<void>();
    const client = createMockApiClient({
      getSystemStatus: vi.fn()
        .mockRejectedValueOnce(new TypeError('Failed to fetch'))
        .mockImplementation(async () => {
          await back.promise;
          return status('2026-09-19T00:00:09Z');
        }),
      restartPanel: vi.fn().mockResolvedValue({ started_at: '2026-09-19T00:00:00Z' }),
    });
    useCommandSession.getState().setOpen(true);
    const reboot = runReboot({ cache, client, t });
    await vi.advanceTimersByTimeAsync(2_000);
    expect(usePanelRestart.getState().restarting).toBe(true);
    expect(await restartPanel({ cache, client, t })).toEqual({ kind: 'busy' });
    back.resolve();
    await vi.advanceTimersByTimeAsync(1_000);
    await reboot;
    expect(usePanelRestart.getState().restarting).toBe(false);
    expect(useCommandSession.getState().entries.map(entry => [entry.text, entry.tone])).toEqual([
      ['Restarting panel…', 'muted'],
      ['Panel restarted', 'success'],
    ]);
  });

  it('gives up after a minute without the restarted panel and says so in a danger toast', async () => {
    vi.useFakeTimers();
    const notify = vi.spyOn(toast, 'add');
    const client = createMockApiClient({ getSystemStatus: vi.fn().mockResolvedValue(status('2026-09-19T00:00:00Z')) });
    const reboot = runReboot({ cache, client, t });
    await vi.advanceTimersByTimeAsync(RESTART_TIMEOUT_MS + 1_000);
    await reboot;
    expect(useCommandSession.getState().entries.at(-1)).toEqual(expect.objectContaining({ text: 'The panel did not come back', tone: 'danger' }));
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ title: 'The panel did not come back', type: 'error' }));
    expect(usePanelRestart.getState().restarting).toBe(false);
  });
});
