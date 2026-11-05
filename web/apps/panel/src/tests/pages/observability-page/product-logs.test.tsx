import { MemoryRouter } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { CoreLogChunk, PanelLog } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { ObservabilityPage } from '@/pages/observability-page/observability-page';

function show(client = createMockApiClient(), url = '/observability') {
  render(
    <MemoryRouter initialEntries={[url]}>
      <ApiClientProvider client={client}>
        <ObservabilityPage />
      </ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}
function waitForAbort(signal?: AbortSignal) {
  return new Promise<void>((resolve) => {
    if (signal?.aborted) resolve();
    else signal?.addEventListener('abort', () => resolve(), { once: true });
  });
}
const file = '2026-09-19-000.log';

describe('unified product logs', () => {
  afterEach(() => vi.useRealTimers());
  it('wires the live toggle to stream cancellation and resumption', async () => {
    const stream = vi.fn(async function* (_file: string, offset = -1, _generation?: string, signal?: AbortSignal) {
      const chunk: CoreLogChunk = {
        file,
        text: offset < 0 ? 'INFO connected\n' : 'ERROR resumed\n',
        generation: 'test-generation', next_offset: 32,
        size: 32,
      };
      yield chunk;
      await waitForAbort(signal);
    });
    show(
      createMockApiClient({
        listCoreLogFiles: vi
          .fn()
          .mockResolvedValue({
            items: [{ name: file, size: 32, deletable: false, updated_at: new Date().toISOString() }],
          }),
        streamCoreLog: stream,
      }),
    );
    await screen.findByText('INFO connected');
    await userEvent.click(screen.getByRole('button', { name: 'Live updates', pressed: true }));
    expect(stream.mock.calls[0]?.[3]?.aborted).toBe(true);
    await userEvent.click(screen.getByRole('button', { name: 'Live updates', pressed: false }));
    await screen.findByText('ERROR resumed');
    expect(stream).toHaveBeenCalledTimes(2);
  });
  it('protects all of today’s segments and deletes only the confirmed historical file', async () => {
    const archive = '2026-09-18-000.log';
    const earlierToday = '2026-09-19-000.log';
    const latest = '2026-09-19-001.log';
    let files = [latest, earlierToday, archive].map((name) => ({ name, size: 32, updated_at: '2026-09-19T00:00:00Z', deletable: name === archive }));
    const client = show(createMockApiClient({
      listCoreLogFiles: vi.fn(async () => ({ items: files })),
      readCoreLog: vi.fn(async (name) => ({ file: name, text: `INFO ${name}\n`, generation: 'test-generation', next_offset: 32, size: 32 })),
      streamCoreLog: vi.fn(async function* (name, _offset, _generation, signal) {
        yield { file: name, text: `INFO ${name}\n`, generation: 'test-generation', next_offset: 32, size: 32 };
        await waitForAbort(signal);
      }),
      deleteCoreLogFile: vi.fn(async (name) => {
        files = files.filter((item) => item.name !== name);
      }),
    }));
    await screen.findByText(`INFO ${latest}`);
    expect(screen.queryByRole('button', { name: 'Delete log file' })).not.toBeInTheDocument();
    for (const name of [earlierToday, archive]) {
      await userEvent.click(screen.getByRole('combobox', { name: 'Log file' }));
      const options = await screen.findAllByRole('option', { name: `${name.slice(0, 10)} · 0.0 MB` });
      await userEvent.click(options[name === earlierToday ? 1 : 0]);
      await screen.findByText(`INFO ${name}`);
      expect(screen.getByRole('button', { name: 'Live updates' })).toBeDisabled();
      if (name === earlierToday) expect(screen.queryByRole('button', { name: 'Delete log file' })).not.toBeInTheDocument();
    }
    await userEvent.click(screen.getByRole('button', { name: 'Delete log file' }));
    expect(screen.getByRole('alertdialog')).toHaveTextContent(archive);
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Cancel' }));
    expect(client.deleteCoreLogFile).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Delete log file' }));
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Delete log file' }));
    await screen.findByText(`INFO ${latest}`);
    expect(client.deleteCoreLogFile).toHaveBeenCalledExactlyOnceWith(archive);
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete log file' })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('combobox', { name: 'Log file' }));
    expect(screen.queryByRole('option', { name: /2026-09-18/ })).not.toBeInTheDocument();
  });
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
      '/observability?tab=panel',
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
    }), '/observability?tab=panel');
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
      '/observability?tab=panel',
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
