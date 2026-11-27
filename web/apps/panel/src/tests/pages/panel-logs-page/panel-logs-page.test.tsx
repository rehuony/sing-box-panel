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
  it('lists panel activity in pages of 50 whose lines open their details from anywhere', async () => {
    const item: LogEntry = {
      id: 'log_1', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'error',
      code: 'future.refresh.failed', message: 'Refresh failed', metadata: { error_code: 'catalog_timeout' },
    };
    const listPanelLogs = vi.fn().mockResolvedValue({ items: [item], total: 120 });
    const client = show(createMockApiClient({ listPanelLogs }));
    const level = await screen.findByRole('cell', { name: 'ERROR' });
    expect(screen.getByRole('button', { name: 'Refresh failed · GitHub did not answer the catalog refresh in time' })).toBeVisible();
    expect(listPanelLogs).toHaveBeenCalledWith(
      expect.objectContaining({ limit: 50, offset: 0 }), expect.any(AbortSignal),
    );
    await userEvent.click(level);
    expect(screen.getByRole('dialog', { name: 'Log details' })).toBeVisible();
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    // Details a press opened leave focus out of the list.
    expect(screen.getByRole('table')).not.toContainElement(document.activeElement as HTMLElement);
    await userEvent.click(screen.getByRole('button', { name: 'Next page' }));
    await waitFor(() => expect(client.listPanelLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 50, offset: 50 }), expect.any(AbortSignal),
    ));
    await userEvent.click(screen.getByRole('combobox', { name: 'Items per page' }));
    await userEvent.click(await screen.findByRole('option', { name: '10 per page' }));
    await waitFor(() => expect(client.listPanelLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 10, offset: 0 }), expect.any(AbortSignal),
    ));
  });
  it('keeps the list one Tab stop that the arrow keys move through and Enter opens', async () => {
    const user = userEvent.setup();
    const entry = (id: string, message: string): LogEntry => ({
      id, time: '2026-09-19T00:00:00Z', source: 'panel', level: 'info', code: id, message, metadata: {},
    });
    show(createMockApiClient({ listPanelLogs: vi.fn().mockResolvedValue({
      items: [entry('first', 'First event'), entry('second', 'Second event')], total: 2,
    }) }));
    const first = await screen.findByRole('button', { name: 'First event' });
    const second = screen.getByRole('button', { name: 'Second event' });
    expect([first.tabIndex, second.tabIndex]).toEqual([0, -1]);
    first.focus();
    await user.keyboard('{ArrowDown}');
    expect(second).toHaveFocus();
    expect([first.tabIndex, second.tabIndex]).toEqual([-1, 0]);
    await user.keyboard('{Enter}');
    expect(screen.getByRole('dialog', { name: 'Log details' })).toBeVisible();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(second).toHaveFocus());
  });
  it('returns to the last valid log page when records disappear during a jump', async () => {
    const item: LogEntry = {
      id: 'retained', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'info',
      code: 'retained', message: 'Retained event', metadata: {},
    };
    const client = show(createMockApiClient({ listPanelLogs: vi.fn()
      .mockResolvedValueOnce({ items: [item], total: 130 })
      .mockResolvedValueOnce({ items: [], total: 62 })
      .mockResolvedValue({ items: [item], total: 62 }),
    }));
    await screen.findByText('Retained event');
    const input = screen.getByRole('spinbutton', { name: 'Current page' });
    fireEvent.change(input, { target: { value: '3' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(client.listPanelLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 50 }), expect.any(AbortSignal),
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
    expect(await screen.findByRole('cell', { name: 'Logs cleared · 3 deleted · 12 KB freed' })).toBeVisible();
  });
});
