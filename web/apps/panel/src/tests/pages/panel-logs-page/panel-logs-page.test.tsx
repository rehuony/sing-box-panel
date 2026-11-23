import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { PanelLog } from '@/api/api-client';

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
    const item: PanelLog = {
      id: 'log_1',
      time: '2026-09-19T00:00:00Z',
      source: 'panel',
      level: 'info',
      code: 'Catalog refreshed',
      message: 'Catalog refreshed',
      status: 'succeeded',
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
    const item: PanelLog = {
      id: 'retained', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'info',
      code: 'retained', message: 'Retained event', status: '', metadata: {},
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
    const items: PanelLog[] = [
      { id: 'operation:done', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'info', status: 'succeeded', code: 'configuration-apply', message: 'Configuration applied', metadata: {} },
      { id: 'operation:failed', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'error', status: 'failed', code: 'catalog-refresh', message: 'Catalog refresh failed', metadata: {} },
      { id: 'security:session', time: '2026-09-19T00:00:00Z', source: 'security', level: 'warn', status: '', code: 'session-renewed', message: 'Administrator session renewed', metadata: {} },
      { id: 'internal-event-id', time: '2026-09-19T00:00:00Z', source: 'panel', level: 'debug', status: '', code: 'saved', message: 'Panel settings saved', metadata: { operation_id: 'internal-operation-id' } },
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
    for (const value of ['succeeded', 'failed', 'canceled', 'internal-operation-id']) {
      expect(within(table).queryByRole('cell', { name: value })).not.toBeInTheDocument();
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
});
