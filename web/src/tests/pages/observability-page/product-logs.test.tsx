import { MemoryRouter } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { CoreLogChunk, PanelLog } from '@/api/api-client';

import { toast } from '@/components/ui/toast-manager';
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
  it('hides output actions when no log files exist', async () => {
    show(createMockApiClient({ listCoreLogFiles: vi.fn().mockResolvedValue({ items: [] }) }));
    await screen.findByText('No core output has been captured.');
    expect(screen.queryByRole('group', { name: 'Log actions' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Live updates' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Clear current log' })).not.toBeInTheDocument();
  });
  it('shows output actions when logs arrive and hides them while filters match nothing', async () => {
    let append = () => {};
    const user = userEvent.setup();
    const client = show(createMockApiClient({
      listCoreLogFiles: vi.fn().mockResolvedValue({
        items: [{ name: file, size: 0, deletable: false, updated_at: new Date().toISOString() }],
      }),
      streamCoreLog: vi.fn(async function* (name, _offset, _generation, signal) {
        yield { file: name, text: '', generation: 'test-generation', next_offset: 0, size: 0 };
        await new Promise<void>((resolve) => {
          append = resolve;
        });
        yield { file: name, text: 'INFO arrived\n', generation: 'test-generation', next_offset: 13, size: 13 };
        await waitForAbort(signal);
      }),
    }));
    await waitFor(() => expect(client.streamCoreLog).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole('group', { name: 'Log actions' })).not.toBeInTheDocument();
    await act(async () => append());
    await screen.findByText('INFO arrived');
    expect(screen.getByRole('group', { name: 'Log actions' })).toBeVisible();
    const search = screen.getByRole('textbox', { name: 'Search displayed output' });
    await user.type(search, 'no match');
    expect(screen.queryByRole('group', { name: 'Log actions' })).not.toBeInTheDocument();
    await user.clear(search);
    expect(screen.getByRole('group', { name: 'Log actions' })).toBeVisible();
    await user.click(screen.getByRole('combobox', { name: 'Log level' }));
    await user.click(await screen.findByRole('option', { name: 'ERROR' }));
    expect(screen.queryByRole('group', { name: 'Log actions' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('combobox', { name: 'Log level' }));
    await user.click(await screen.findByRole('option', { name: 'ALL' }));
    expect(screen.getByRole('group', { name: 'Log actions' })).toBeVisible();
  });
  it('replaces toolbar controls when switching between live and panel logs', async () => {
    const user = userEvent.setup();
    show();
    expect(screen.getByRole('textbox', { name: 'Search displayed output' })).toBeVisible();
    await user.click(screen.getByRole('tab', { name: 'Panel logs' }));
    expect(screen.getByRole('textbox', { name: 'Search event names, messages or codes' })).toBeVisible();
    expect(screen.getAllByRole('textbox')).toHaveLength(1);
    expect(screen.queryByRole('combobox', { name: 'Log file' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Live logs' }));
    expect(screen.getByRole('textbox', { name: 'Search displayed output' })).toBeVisible();
    expect(screen.queryByRole('textbox', { name: 'Search event names, messages or codes' })).not.toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Log file' })).toBeVisible();
  });
  it('keeps paused output on file rotation and lets the toolbar toggle resume the latest file', async () => {
    vi.useFakeTimers();
    let latest = file;
    const client = createMockApiClient({
      listCoreLogFiles: vi.fn(async () => ({
        items: [{ name: latest, size: 32, deletable: false, updated_at: new Date().toISOString() }],
      })),
      streamCoreLog: vi.fn(async function* (name, _offset, _generation, signal) {
        yield { file: name, text: `INFO ${name}\n`, generation: 'test-generation', next_offset: 32, size: 32 };
        await waitForAbort(signal);
      }),
    });
    show(client);
    await act(async () => {});
    fireEvent.click(screen.getByRole('button', { name: 'Live updates', pressed: true }));
    latest = '2026-09-19-001.log';
    await act(() => vi.advanceTimersByTimeAsync(10_000));
    expect(screen.getByText(`INFO ${file}`)).toBeVisible();
    expect(client.streamCoreLog).toHaveBeenCalledTimes(1);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Live updates', pressed: false }));
    });
    expect(screen.getByText(`INFO ${latest}`)).toBeVisible();
    expect(screen.queryByText(`INFO ${file}`)).not.toBeInTheDocument();
    expect(client.streamCoreLog).toHaveBeenLastCalledWith(latest, -1, '', expect.any(AbortSignal));
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
  it('toggles live output with the toolbar toggle and resumes at the last received byte', async () => {
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
    expect(screen.getAllByRole('tab')).toHaveLength(2);
    await screen.findByText('INFO connected');
    const live = screen.getByRole('button', { name: 'Live updates', pressed: true });
    await userEvent.click(live);
    const paused = screen.getByRole('button', { name: 'Live updates', pressed: false });
    expect(paused).toHaveFocus();
    expect(stream.mock.calls[0]?.[3]?.aborted).toBe(true);
    await userEvent.keyboard('{Enter}');
    await screen.findByText('ERROR resumed');
    expect(screen.getByRole('button', { name: 'Live updates', pressed: true })).toHaveFocus();
    expect(stream.mock.calls.at(-1)?.[1]).toBe(32);
    expect(screen.getAllByText('INFO connected')).toHaveLength(1);
    await userEvent.click(screen.getByRole('combobox', { name: 'Log level' }));
    await userEvent.click(await screen.findByRole('option', { name: 'ERROR' }));
    expect(screen.queryByText('INFO connected')).not.toBeInTheDocument();
    expect(screen.getByText('ERROR resumed')).toBeVisible();
  });
  it('clears persisted output while paused despite filters and keeps it cleared on remount', async () => {
    let saved = 'INFO before clear\nERROR hidden output\n';
    const client = show(createMockApiClient({
      listCoreLogFiles: vi.fn(async () => ({
        items: [{ name: file, size: saved.length, deletable: false, updated_at: new Date().toISOString() }],
      })),
      streamCoreLog: vi.fn(async function* (name, offset, _generation, signal) {
        yield {
          file: name, text: saved.slice(Math.max(0, offset ?? -1)), generation: 'test-generation', next_offset: saved.length, size: saved.length,
        };
        await waitForAbort(signal);
      }),
      clearCoreLog: vi.fn(async () => {
        saved = '';
      }),
    }));
    await screen.findByText('INFO before clear');
    await userEvent.click(screen.getByRole('button', { name: 'Live updates' }));
    await userEvent.type(screen.getByRole('textbox', { name: 'Search displayed output' }), 'before clear');
    await userEvent.click(screen.getByRole('combobox', { name: 'Log level' }));
    await userEvent.click(await screen.findByRole('option', { name: 'INFO' }));
    await userEvent.click(screen.getByRole('button', { name: 'Clear current log' }));
    expect(screen.getByRole('alertdialog')).toHaveTextContent(file);
    expect(screen.getByRole('alertdialog')).toHaveTextContent('including records hidden by filters');
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Cancel' }));
    expect(client.clearCoreLog).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Clear current log' }));
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Clear current log' }));
    expect(await screen.findByText('Saved log output cleared')).toBeVisible();
    expect(client.clearCoreLog).toHaveBeenCalledExactlyOnceWith(file);
    expect(client.deleteCoreLogFile).not.toHaveBeenCalled();
    expect(saved).toBe('');
    expect(screen.queryByRole('group', { name: 'Log actions' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Resume live output' })).toBeEnabled();
    expect(screen.getByRole('textbox', { name: 'Search displayed output' })).toHaveValue('before clear');
    await userEvent.clear(screen.getByRole('textbox', { name: 'Search displayed output' }));
    saved = 'INFO after clear\n';
    await userEvent.click(screen.getByRole('button', { name: 'Resume live output' }));
    await screen.findByText('INFO after clear');
    expect(screen.getByRole('group', { name: 'Log actions' })).toBeVisible();
    expect(client.streamCoreLog).toHaveBeenLastCalledWith(file, 0, '', expect.any(AbortSignal));
    cleanup();
    show(client);
    await screen.findByText('INFO after clear');
    expect(screen.queryByText('INFO before clear')).not.toBeInTheDocument();
    expect(screen.queryByText('ERROR hidden output')).not.toBeInTheDocument();
  });
  it('rejects queued stream output during clear and resumes from zero after success', async () => {
    let releaseOld = () => {};
    let finishClear = () => {};
    let saved = 'INFO first\n';
    const client = show(createMockApiClient({
      listCoreLogFiles: vi.fn(async () => ({
        items: [{ name: file, size: saved.length, deletable: false, updated_at: new Date().toISOString() }],
      })),
      streamCoreLog: vi.fn(async function* (name, offset, _generation, signal) {
        yield { file: name, text: saved, generation: 'test-generation', next_offset: saved.length, size: saved.length };
        if (offset === -1) {
          await new Promise<void>((resolve) => {
            releaseOld = resolve;
          });
          yield { file: name, text: 'INFO stale chunk\n', generation: 'test-generation', next_offset: 100, size: 100 };
        }
        await waitForAbort(signal);
      }),
      clearCoreLog: vi.fn(async () => {
        await new Promise<void>((resolve) => {
          finishClear = resolve;
        });
        saved = 'INFO new output\n';
      }),
    }));
    await screen.findByText('INFO first');
    const output = screen.getByRole('region', { name: 'Live logs' });
    Object.defineProperties(output, {
      scrollHeight: { value: 1000, configurable: true }, clientHeight: { value: 100 },
    });
    fireEvent.scroll(output, { target: { scrollTop: 100 } });
    await userEvent.click(screen.getByRole('button', { name: 'Scroll to bottom' }));
    expect(output.scrollTop).toBe(1000);
    await userEvent.click(screen.getByRole('button', { name: 'Clear current log' }));
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Clear current log' }));
    expect(client.streamCoreLog).toHaveBeenCalledTimes(1);
    expect(vi.mocked(client.streamCoreLog).mock.calls[0][3]?.aborted).toBe(true);
    expect(screen.getByRole('button', { name: 'Clearing…' })).toBeDisabled();
    await act(async () => releaseOld());
    expect(screen.queryByText('INFO stale chunk')).not.toBeInTheDocument();
    expect(screen.getByText('INFO first')).toBeVisible();
    await act(async () => finishClear());
    expect(await screen.findByText('INFO new output')).toBeVisible();
    expect(screen.queryByText('INFO first')).not.toBeInTheDocument();
    expect(client.streamCoreLog).toHaveBeenLastCalledWith(file, 0, '', expect.any(AbortSignal));
    expect(output.scrollTop).toBe(1000);
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });
  it('preserves output and resumes the old cursor when clearing fails', async () => {
    const notification = vi.spyOn(toast, 'add');
    const client = show(createMockApiClient({
      listCoreLogFiles: vi.fn().mockResolvedValue({
        items: [{ name: file, size: 32, deletable: false, updated_at: new Date().toISOString() }],
      }),
      streamCoreLog: vi.fn(async function* (name, offset, _generation, signal) {
        yield { file: name, text: offset === -1 ? 'INFO preserved\n' : '', generation: 'test-generation', next_offset: 32, size: 32 };
        await waitForAbort(signal);
      }),
      clearCoreLog: vi.fn().mockRejectedValue(new Error('Clear failed')),
    }));
    await screen.findByText('INFO preserved');
    await userEvent.click(screen.getByRole('button', { name: 'Clear current log' }));
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Clear current log' }));
    await waitFor(() => expect(notification).toHaveBeenCalledWith(expect.objectContaining({ title: 'Could not clear the log file', type: 'error' })));
    expect(screen.getByText('INFO preserved')).toBeVisible();
    expect(screen.getByRole('alertdialog')).toBeVisible();
    expect(screen.queryByText('Saved log output cleared')).not.toBeInTheDocument();
    expect(client.streamCoreLog).toHaveBeenLastCalledWith(file, 32, 'test-generation', expect.any(AbortSignal));
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
  it('retains the selected file on deletion failure and handles deleting the last file', async () => {
    let exists = true;
    const client = show(createMockApiClient({
      listCoreLogFiles: vi.fn(async () => ({ items: exists ? [{ name: file, size: 32, deletable: true, updated_at: '2026-09-19T00:00:00Z' }] : [] })),
      streamCoreLog: vi.fn(async function* (name, _offset, _generation, signal) {
        yield { file: name, text: 'INFO preserved\n', generation: 'test-generation', next_offset: 32, size: 32 };
        await waitForAbort(signal);
      }),
      deleteCoreLogFile: vi.fn().mockRejectedValueOnce(new Error('Deletion failed')).mockImplementationOnce(async () => {
        exists = false;
      }),
    }));
    await screen.findByText('INFO preserved');
    await userEvent.click(screen.getByRole('button', { name: 'Delete log file' }));
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Delete log file' }));
    await waitFor(() => expect(client.deleteCoreLogFile).toHaveBeenCalledTimes(1));
    expect(screen.getByText('INFO preserved')).toBeVisible();
    expect(screen.getByRole('alertdialog')).toBeVisible();
    await userEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Delete log file' }));
    await screen.findByText('No core output has been captured.');
    expect(screen.queryByText('INFO preserved')).not.toBeInTheDocument();
    expect(screen.queryByRole('group', { name: 'Log actions' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Live updates' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Clear current log' })).not.toBeInTheDocument();
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
    expect(screen.getAllByRole('columnheader').map((header) => header.textContent))
      .toEqual(['Occurred at', 'Level', 'Event summary', 'Event source', 'Actions']);
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
