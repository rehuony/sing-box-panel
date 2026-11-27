import { MemoryRouter } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';

import type { CoreLogChunk, NodeRuntimeTransition } from '@/api/api-client';

import i18n from '@/i18n';
import { ApiClientProvider } from '@/api/api-client-context';
import { CurrentNodeContext } from '@/stores/current-node.store';
import { NodeLogsPage } from '@/pages/node-logs-page/node-logs-page';
import { createMockApiClient, testNode } from '@/tests/api/mock-api-client';

function show(client = createMockApiClient(), url = `/nodes/${testNode.id}/logs#logs-core`) {
  render(
    <MemoryRouter initialEntries={[url]}>
      <ApiClientProvider client={client}>
        <CurrentNodeContext value={testNode}>
          <NodeLogsPage />
        </CurrentNodeContext>
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

describe('node core logs', () => {
  afterEach(async () => {
    vi.useRealTimers();
    await i18n.changeLanguage('en');
  });
  it('opens on the node\'s events and clears only them', async () => {
    const user = userEvent.setup();
    const client = show(createMockApiClient({
      clearPanelLogs: vi.fn().mockResolvedValue({ deleted: 2, freed_bytes: 4096 }),
      getLogUsage: vi.fn().mockResolvedValue({ entries: 2, bytes: 20_480 }),
      listPanelLogs: vi.fn().mockResolvedValue({ items: [{
        id: 'log:one', time: '2026-09-24T01:16:57Z', source: 'panel', level: 'info',
        code: 'runtime.start.completed', message: 'Core start completed', metadata: { node_id: testNode.id },
      }], total: 1 }),
    }), `/nodes/${testNode.id}/logs`);
    expect(await screen.findByRole('cell', { name: 'Core start operation completed' })).toBeVisible();
    expect(client.listPanelLogs).toHaveBeenCalledWith(
      expect.objectContaining({ scope: { nodeID: testNode.id } }), expect.any(AbortSignal),
    );
    expect(screen.queryByRole('columnheader', { name: 'Event source' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Clear node events' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Clear node events' });
    expect(await within(dialog).findByText(/This deletes this node's 2 events/)).toBeVisible();
    await user.click(within(dialog).getByRole('button', { name: 'Clear' }));
    expect(client.clearPanelLogs).toHaveBeenCalledExactlyOnceWith({ nodeID: testNode.id });
  });
  it('lists the node\'s runtime states and their details without confusing them with operations', async () => {
    const transition: NodeRuntimeTransition = {
      id: 2, node_id: testNode.id, node_name: testNode.name, state: 'running', reason: 'start_succeeded',
      occurred_at: '2026-09-24T01:16:57Z', pid: 123, process_started_at: '2026-09-24T01:16:57Z',
      activation_bundle_id: 'bundle_current',
    };
    const client = show(createMockApiClient({
      listFleetRuntimeHistory: vi.fn().mockResolvedValue({ items: [transition], total: 1 }),
    }));
    await userEvent.click(screen.getByRole('tab', { name: 'Runtime states' }));
    expect(await screen.findByRole('cell', { name: 'Core started' })).toBeVisible();
    expect(client.listFleetRuntimeHistory).toHaveBeenCalledWith(
      expect.objectContaining({ nodeID: testNode.id }), expect.any(AbortSignal),
    );
    expect(screen.getByRole('cell', { name: 'Running' })).toBeVisible();
    expect(screen.queryByText(/bundle_current/)).not.toBeInTheDocument();
    await act(() => i18n.changeLanguage('zh-CN'));
    expect(screen.queryByText('start_succeeded')).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('table').querySelector('time')).toHaveAttribute('datetime', transition.occurred_at));
    await userEvent.click(screen.getByRole('button', { name: '核心已启动' }));
    const dialog = screen.getByRole('dialog', { name: '运行状态详情' });
    expect(within(dialog).getByText(testNode.name)).toBeVisible();
    const context = within(dialog).getByRole('region', { name: '事件上下文' });
    expect(within(context).getByText('进程 ID')).toBeVisible();
    expect(within(context).getByText('bundle_current')).toBeVisible();
  });
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
});
