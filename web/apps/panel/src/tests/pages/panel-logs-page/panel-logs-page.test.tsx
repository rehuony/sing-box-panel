import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { toast } from '@shared/components/ui/toast-manager';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { LogEntry } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { PanelLogsPage } from '@/pages/panel-logs-page/panel-logs-page';

function show(client = createMockApiClient(), url = '/logs') {
  render(
    <MemoryRouter initialEntries={[url]}>
      <ApiClientProvider client={client}>
        <PanelLogsPage />
      </ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}

describe('panel logs', () => {
  it('lists panel activity in five columns with details and paginates', async () => {
    const item: LogEntry = {
      id: 'log_1',
      time: '2026-09-19T00:00:00Z',
      source: 'panel',
      level: 'info',
      code: 'Catalog refreshed',
      message: 'Catalog refreshed',
      metadata: {},
    };
    const client = show(
      createMockApiClient({
        listPanelLogs: vi
          .fn()
          .mockResolvedValueOnce({ items: [item], total: 11 })
          .mockResolvedValue({ items: [item], total: 11 }),
      }),
    );
    expect(await screen.findByRole('cell', { name: 'INFO' })).toBeVisible();

    expect(screen.getByRole('button', { name: 'View details: Catalog refreshed' })).toBeVisible();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Next page' }));
    await waitFor(() =>
      expect(client.listPanelLogs).toHaveBeenCalledWith(
        expect.objectContaining({ offset: 10 }),
        expect.any(AbortSignal),
      ),
    );
    await userEvent.click(screen.getByRole('combobox', { name: 'Items per page' }));
    await userEvent.keyboard('[ArrowDown]');
    await userEvent.click(await screen.findByRole('option', { name: '50 per page' }));
    await waitFor(() =>
      expect(client.listPanelLogs).toHaveBeenLastCalledWith(
        expect.objectContaining({ limit: 50, offset: 0 }),
        expect.any(AbortSignal),
      ),
    );
  });
  it('returns to the last valid log page when records disappear during a jump', async () => {
    const item: LogEntry = {
      id: 'retained', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'info',
      code: 'retained', message: 'Retained event', metadata: {},
    };
    const client = show(createMockApiClient({ listPanelLogs: vi.fn()
      .mockResolvedValueOnce({ items: [item], total: 30 })
      .mockResolvedValueOnce({ items: [], total: 12 })
      .mockResolvedValue({ items: [item], total: 12 }),
    }));
    await screen.findByText('Retained event');
    const input = screen.getByRole('spinbutton', { name: 'Current page' });
    fireEvent.change(input, { target: { value: '3' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(client.listPanelLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 10 }), expect.any(AbortSignal),
    ));
    expect(await screen.findByText('Retained event')).toBeVisible();
    expect(input).toHaveValue(2);
    expect(input).toHaveAccessibleDescription('2 pages in total');
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
  });

  it('uses the filtered log level for operation and security events', async () => {
    const items: LogEntry[] = [
      { id: 'operation:done', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'info', code: 'configuration-apply', message: 'Configuration applied', metadata: {} },
      { id: 'operation:failed', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'error', code: 'catalog-refresh', message: 'Catalog refresh failed', metadata: {} },
      { id: 'security:session', time: '2026-09-19T00:00:00Z', source: 'security', level: 'warn', code: 'session-renewed', message: 'Administrator session renewed', metadata: {} },
      { id: 'internal-event-id', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'debug', code: 'saved', message: 'Panel settings saved', metadata: { operation_id: 'internal-operation-id' } },
    ];
    const client = show(
      createMockApiClient({ listPanelLogs: vi.fn(async (filter) => ({
        items: items.filter((item) => !filter?.level || item.level === filter.level),
        total: items.filter((item) => !filter?.level || item.level === filter.level).length,
      })) }),
    );
    expect(await screen.findByText('Panel settings saved')).toBeVisible();
    const table = screen.getByRole('table');
    for (const level of ['INFO', 'ERROR', 'WARN', 'DEBUG']) {
      expect(within(table).getByRole('cell', { name: level })).toBeVisible();
    }
    expect(table).not.toHaveTextContent('internal-operation-id');
    await userEvent.click(screen.getByRole('combobox', { name: 'Log level' }));
    await userEvent.click(await screen.findByRole('option', { name: 'ERROR' }));
    await waitFor(() => expect(within(table).getAllByRole('row')).toHaveLength(2));
    expect(within(table).getByRole('cell', { name: 'ERROR' })).toBeVisible();
    expect(client.listPanelLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ level: 'error' }),
      expect.any(AbortSignal),
    );
  });

  it('clears the panel\'s own entries after confirming what they hold, then lists the entry the clearing recorded', async () => {
    const user = userEvent.setup();
    const notify = vi.spyOn(toast, 'add');
    const cleared: LogEntry = {
      id: 'log_cleared', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'info',
      code: 'logs.cleared', message: 'Panel log cleared', metadata: { deleted: 3, freed_bytes: 12_288 },
    };
    const client = show(createMockApiClient({
      clearPanelLogs: vi.fn().mockResolvedValue({ deleted: 3, freed_bytes: 12_288 }),
      getLogUsage: vi.fn().mockResolvedValue({ entries: 3, bytes: 20_480 }),
      listPanelLogs: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    }));
    await waitFor(() => expect(client.listPanelLogs).toHaveBeenCalledWith(
      expect.objectContaining({ scope: { panel: true } }), expect.any(AbortSignal),
    ));
    await user.click(await screen.findByRole('combobox', { name: 'Log level' }));
    await user.click(await screen.findByRole('option', { name: 'ERROR' }));
    await user.click(screen.getByRole('button', { name: 'Clear' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Clear panel logs' });
    expect(await within(dialog).findByText('This deletes the panel\'s 3 own log entries; node events stay. All logs take 20 KB on disk.')).toBeVisible();
    expect(client.getLogUsage).toHaveBeenCalledWith({ panel: true }, expect.any(AbortSignal));
    expect(within(dialog).getByText('Runtime states are not affected. This cannot be undone.')).toBeVisible();
    vi.mocked(client.listPanelLogs).mockResolvedValue({ items: [cleared], total: 1 });
    await user.click(within(dialog).getByRole('button', { name: 'Clear' }));
    expect(client.clearPanelLogs).toHaveBeenCalledExactlyOnceWith({ panel: true });
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ title: 'Cleared 3 log entries, freed 12 KB', type: 'success' }));
    expect(await screen.findByRole('cell', { name: 'Logs cleared' })).toBeVisible();
  });
});
