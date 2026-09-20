import { MemoryRouter } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, renderHook, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { CoreLogChunk, PanelLog } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { useCoreLogs } from '@/pages/observability-page/use-core-logs';
import { createMockApiClient, testTask } from '@/tests/api/mock-api-client';
import { PanelLogDetail } from '@/pages/observability-page/panel-log-detail';
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
  it('does not change the displayed file on rotation while paused', async () => {
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
    const { result } = renderHook(() => useCoreLogs(), {
      wrapper: ({ children }) => <ApiClientProvider client={client}>{children}</ApiClientProvider>,
    });
    await act(async () => {});
    act(() => result.current.setPaused(true));
    latest = '2026-09-19-001.log';
    await act(() => vi.advanceTimersByTimeAsync(10_000));
    expect(result.current.file).toBe(file);
    expect(result.current.text).toContain(file);
    await act(async () => result.current.setPaused(false));
    expect(result.current.file).toBe(latest);
    expect(result.current.text).toBe(`INFO ${latest}\n`);
  });
  it('recovers polling errors and stops polling terminal tasks', async () => {
    vi.useFakeTimers();
    const client = createMockApiClient({
      getTask: vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ ...testTask, status: 'succeeded' }),
    });
    render(
      <ApiClientProvider client={client}>
        <PanelLogDetail entry={null} taskID={testTask.id} onClose={() => {}} onTaskChange={() => {}} />
      </ApiClientProvider>,
    );
    await act(async () => {});
    expect(screen.getByRole('alert')).toBeVisible();
    await act(() => vi.advanceTimersByTimeAsync(2000));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByText('succeeded')).toBeVisible();
    await act(() => vi.advanceTimersByTimeAsync(10_000));
    expect(client.getTask).toHaveBeenCalledTimes(2);
  });
  it('ignores a retry response after its dialog closes', async () => {
    let finish!: (value: typeof testTask) => void;
    const retryTask = vi.fn(() => new Promise<typeof testTask>((resolve) => {
      finish = resolve;
    }));
    const client = createMockApiClient({ getTask: vi.fn().mockResolvedValue({ ...testTask, status: 'failed', kind: 'catalog-refresh' }), retryTask });
    show(client, `/observability?tab=panel&task=${testTask.id}`);
    await userEvent.click(await screen.findByRole('button', { name: 'Retry' }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close' }));
    await act(async () => finish({ ...testTask, id: 'retry-late', status: 'queued' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(client.getTask).toHaveBeenCalledTimes(1);
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
  it('shows only two tabs and resumes at the last received byte after pause', async () => {
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
    await userEvent.click(screen.getByRole('button', { name: 'Pause live output' }));
    expect(screen.getByText('Paused')).toBeVisible();
    await userEvent.click(screen.getByRole('button', { name: 'Resume live output' }));
    await screen.findByText('ERROR resumed');
    expect(stream.mock.calls.at(-1)?.[1]).toBe(32);
    expect(screen.getAllByText('INFO connected')).toHaveLength(1);
    await userEvent.click(screen.getByRole('combobox', { name: 'Log level' }));
    await userEvent.click(await screen.findByRole('option', { name: 'ERROR' }));
    expect(screen.queryByText('INFO connected')).not.toBeInTheDocument();
    expect(screen.getByText('ERROR resumed')).toBeVisible();
  });
  it('lists panel activity without a related task column, paginates and opens centered details', async () => {
    const item: PanelLog = {
      id: `task:${testTask.id}`,
      task_id: testTask.id,
      time: testTask.updated_at,
      source: 'task',
      level: 'info',
      code: testTask.kind,
      message: testTask.kind,
      status: testTask.status,
      metadata: {},
    };
    const client = show(
      createMockApiClient({
        listPanelLogs: vi
          .fn()
          .mockResolvedValueOnce({ items: [item], next: { time: item.time, id: item.id } })
          .mockResolvedValue({ items: [item] }),
      }),
      '/observability?tab=panel',
    );
    const details = await screen.findByRole('button', { name: 'Details' });
    expect(screen.queryByRole('columnheader', { name: 'Related task' })).not.toBeInTheDocument();
    await userEvent.click(details);
    expect(await screen.findByRole('dialog')).toBeVisible();
    expect(await within(screen.getByRole('dialog')).findByText('The operation completed successfully.')).toBeVisible();
    expect(within(screen.getByRole('dialog')).queryByText(testTask.id)).not.toBeInTheDocument();
    await userEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Close' }),
    );
    await userEvent.click(screen.getByRole('button', { name: 'Next' }));
    await waitFor(() =>
      expect(client.listPanelLogs).toHaveBeenCalledWith(
        expect.objectContaining({ beforeID: item.id }),
        expect.any(AbortSignal),
      ),
    );
    await userEvent.click(screen.getByRole('combobox', { name: 'Page size' }));
    await userEvent.keyboard('[ArrowDown]');
    await userEvent.click(await screen.findByRole('option', { name: '50 per page' }));
    await waitFor(() =>
      expect(client.listPanelLogs).toHaveBeenLastCalledWith(
        expect.objectContaining({ limit: 50, beforeID: undefined }),
        expect.any(AbortSignal),
      ),
    );
  });
  it.each(['succeeded', 'failed'] as const)('shows readable %s guidance without raw task data', async (status) => {
    const client = createMockApiClient({
      getTask: vi.fn().mockResolvedValue({
        ...testTask,
        status,
        result: { artifact_id: 'internal-artifact-id' },
        failure: { code: 'handler_failed', message: 'internal-error-detail' },
      }),
    });
    render(
      <ApiClientProvider client={client}>
        <PanelLogDetail entry={null} taskID={testTask.id} onClose={() => {}} onTaskChange={() => {}} />
      </ApiClientProvider>,
    );
    const guidance = status === 'succeeded'
      ? 'The operation completed successfully.'
      : 'The operation failed. Review the relevant settings and logs before trying again.';
    expect(await screen.findByText(guidance)).toBeVisible();
    const dialog = screen.getByRole('dialog');
    for (const value of [testTask.id, 'internal-artifact-id', 'handler_failed', 'internal-error-detail']) {
      expect(dialog).not.toHaveTextContent(value);
    }
    expect(dialog.querySelector('pre')).toBeNull();
  });
  it('shows an event message without internal metadata', async () => {
    render(
      <ApiClientProvider client={createMockApiClient()}>
        <PanelLogDetail
          entry={{ id: 'internal-event-id', time: testTask.updated_at, source: 'panel', level: 'info', status: '', code: 'saved', message: 'Panel settings saved', metadata: { task_id: 'internal-task-id' } }}
          taskID={null} onClose={() => {}} onTaskChange={() => {}}
        />
      </ApiClientProvider>,
    );
    expect(await screen.findByText('Panel settings saved')).toBeVisible();
    expect(screen.getByRole('dialog')).not.toHaveTextContent('internal-task-id');
  });
});
