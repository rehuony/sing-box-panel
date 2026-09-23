import { MemoryRouter } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { CoreLogChunk, PanelLog } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { ObservabilityPage } from '@/pages/observability-page/observability-page';
import { appendCoreText, parseCoreLines } from '@/pages/observability-page/core-log-lines';

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
  it('replaces toolbar controls when switching between live and panel logs', async () => {
    const user = userEvent.setup();
    show();
    expect(screen.getByRole('textbox', { name: 'Search displayed output' })).toBeVisible();
    await user.click(screen.getByRole('tab', { name: 'Panel logs' }));
    expect(screen.getByRole('textbox', { name: 'Search panel messages' })).toBeVisible();
    expect(screen.getAllByRole('textbox')).toHaveLength(1);
    expect(screen.queryByRole('combobox', { name: 'Log file' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Live logs' }));
    expect(screen.getByRole('textbox', { name: 'Search displayed output' })).toBeVisible();
    expect(screen.queryByRole('textbox', { name: 'Search panel messages' })).not.toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Log file' })).toBeVisible();
  });
  it('keeps paused output on file rotation and lets the status pill resume the latest file', async () => {
    vi.useFakeTimers();
    let latest = file;
    const client = createMockApiClient({
      listCoreLogFiles: vi.fn(async () => ({
        items: [{ name: latest, size: 32, updated_at: new Date().toISOString() }],
      })),
      streamCoreLog: vi.fn(async function* (name, _offset, signal) {
        yield { file: name, text: `INFO ${name}\n`, next_offset: 32, size: 32 };
        await waitForAbort(signal);
      }),
    });
    show(client);
    await act(async () => {});
    fireEvent.click(screen.getByRole('button', { name: 'Live', pressed: true }));
    latest = '2026-09-19-001.log';
    await act(() => vi.advanceTimersByTimeAsync(10_000));
    expect(screen.getByText(`INFO ${file}`)).toBeVisible();
    expect(client.streamCoreLog).toHaveBeenCalledTimes(1);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Paused', pressed: false }));
    });
    expect(screen.getByText(`INFO ${latest}`)).toBeVisible();
    expect(screen.queryByText(`INFO ${file}`)).not.toBeInTheDocument();
    expect(client.streamCoreLog).toHaveBeenLastCalledWith(latest, -1, expect.any(AbortSignal));
  });
  it('keeps message colors with levels, including multiline errors, and bounds the buffer', () => {
    const lines = parseCoreLines(
      '+0800 2026-09-19 12:00:00 ERROR TLS handshake\nEOF\nINFO connected\n',
    );
    expect(lines[0]).toMatchObject({
      prefix: '+0800 2026-09-19 12:00:00 ',
      message: 'ERROR TLS handshake',
      level: 'error',
    });
    expect(lines[1].level).toBe('error');
    expect(lines[2].level).toBe('info');
    expect(appendCoreText('old\n'.repeat(2500), 'INFO newest\n').split('\n')).toHaveLength(2001);
  });
  it('toggles live output with the status pill and resumes at the last received byte', async () => {
    const stream = vi.fn(async function* (_file: string, offset = -1, signal?: AbortSignal) {
      const chunk: CoreLogChunk = {
        file,
        text: offset < 0 ? 'INFO connected\n' : 'ERROR resumed\n',
        next_offset: 32,
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
            items: [{ name: file, size: 32, updated_at: new Date().toISOString() }],
          }),
        streamCoreLog: stream,
      }),
    );
    expect(screen.getAllByRole('tab')).toHaveLength(2);
    await screen.findByText('INFO connected');
    const live = screen.getByRole('button', { name: 'Live', pressed: true });
    await userEvent.click(live);
    const paused = screen.getByRole('button', { name: 'Paused', pressed: false });
    expect(paused).toHaveFocus();
    expect(stream.mock.calls[0]?.[2]?.aborted).toBe(true);
    await userEvent.keyboard('{Enter}');
    await screen.findByText('ERROR resumed');
    expect(screen.getByRole('button', { name: 'Live', pressed: true })).toHaveFocus();
    expect(stream.mock.calls.at(-1)?.[1]).toBe(32);
    expect(screen.getAllByText('INFO connected')).toHaveLength(1);
    await userEvent.click(screen.getByRole('combobox', { name: 'Log level' }));
    await userEvent.click(await screen.findByRole('option', { name: 'ERROR' }));
    expect(screen.queryByText('INFO connected')).not.toBeInTheDocument();
    expect(screen.getByText('ERROR resumed')).toBeVisible();
  });
  it('lists panel activity in four columns without detail actions and paginates', async () => {
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
    expect(screen.getAllByRole('columnheader').map((header) => header.textContent))
      .toEqual(['Time', 'Message', 'Log level', 'Source']);
    expect(screen.queryByRole('button', { name: 'Details' })).not.toBeInTheDocument();
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
