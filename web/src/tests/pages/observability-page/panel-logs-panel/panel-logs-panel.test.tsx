import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { PanelLog } from '@/api/api-client';

import i18n from '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { PanelLogsPanel } from '@/pages/observability-page/panel-logs-panel';
import { matchingEventCodes } from '@/pages/observability-page/panel-logs-panel/panel-log-presentation';

const item: PanelLog = {
  id: 'log:one', time: '2026-09-24T01:16:57.123456Z', source: 'panel', level: 'info',
  code: 'runtime.start.completed', message: 'Core start completed', status: '', metadata: {},
};
function show(client = createMockApiClient({ listPanelLogs: vi.fn().mockResolvedValue({ items: [item], total: 1 }) })) {
  render(<ApiClientProvider client={client}><PanelLogsPanel /></ApiClientProvider>);
  return client;
}

afterEach(async () => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  await i18n.changeLanguage('en');
});

describe('panel log presentation and details', () => {
  it('localizes known events without confusing action completion with runtime state', async () => {
    show(createMockApiClient({ listPanelLogs: vi.fn().mockResolvedValue({ items: [
      item,
      { ...item, id: 'runtime:2', code: 'start_succeeded', message: 'start_succeeded', source: 'runtime', status: 'running', metadata: { pid: 123, activation_bundle_id: 'bundle_current' } },
      { ...item, id: 'log:3', code: 'future.event', message: 'A future event' },
    ], total: 3 }) }));
    expect(await screen.findByText('Core start operation completed')).toBeVisible();
    expect(screen.getByText('Core started')).toBeVisible();
    expect(screen.getByRole('cell', { name: 'Core started' })).toBeVisible();
    expect(screen.queryByText(/bundle_current/)).not.toBeInTheDocument();
    expect(screen.getByText('A future event')).toBeVisible();
    await act(() => i18n.changeLanguage('zh-CN'));
    expect(screen.getByText('核心启动操作完成')).toBeVisible();
    expect(screen.getByText('核心已启动')).toBeVisible();
    expect(screen.queryByText(/进程 ID/)).not.toBeInTheDocument();
    expect(screen.queryByText('start_succeeded')).not.toBeInTheDocument();
    expect(screen.getByText('A future event')).toBeVisible();
    expect(screen.getByRole('table').querySelector('time')).toHaveAttribute('datetime', item.time);
    await userEvent.click(screen.getByRole('button', { name: '查看详情: 核心已启动' }));
    const context = screen.getByRole('region', { name: '事件上下文' });
    expect(within(context).getByText('进程 ID')).toBeVisible();
    expect(within(context).getByText('123')).toBeVisible();
    expect(within(context).getByText('bundle_current')).toBeVisible();
  });

  it('shows complete context and raw data, copies the original record and restores focus', async () => {
    const user = userEvent.setup();
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
    const notify = vi.spyOn(toast, 'add');
    const record: PanelLog = { ...item, status: 'running', metadata: {
      duration_ms: 0, enabled: false, missing: null, nested: { values: [0, false, 'safe'] },
      custom_field: '<img src=x onerror=alert(1)>', process_started_at: item.time,
    } };
    const client = show(createMockApiClient({
      listPanelLogs: vi.fn().mockResolvedValue({ items: [record], total: 1 }),
    }));
    const trigger = await screen.findByRole('button', { name: 'View details: Core start operation completed' });
    await user.click(trigger);
    const dialog = screen.getByRole('dialog', { name: 'Log details' });
    expect(within(dialog).getByText('Running')).toBeVisible();
    expect(within(dialog).getByText('0', { exact: true })).toBeVisible();
    expect(within(dialog).getByText('false', { exact: true })).toBeVisible();
    expect(within(dialog).getByText('null', { exact: true })).toBeVisible();
    expect(within(dialog).getByText('custom_field')).toBeVisible();
    expect(within(dialog).getByText('<img src=x onerror=alert(1)>')).toBeVisible();
    expect(dialog.querySelector('img')).toBeNull();
    expect(within(dialog).queryByRole('region', { name: 'Original message' })).not.toBeInTheDocument();
    expect(dialog.querySelector('time')?.textContent).toMatch(/\.123.*GMT/);
    expect(dialog.querySelector('details')).not.toHaveAttribute('open');
    await user.click(within(dialog).getByText('Original record', { selector: 'summary' }));
    expect(dialog.querySelector('pre')).toHaveTextContent('runtime.start.completed');
    expect(dialog.querySelector('pre')).toHaveTextContent('Core start completed');
    await user.click(within(dialog).getByRole('button', { name: 'Copy log' }));
    expect(copy).toHaveBeenCalledWith(JSON.stringify(record, null, 2));
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ title: 'Log copied', type: 'success' }));
    expect(client.getLog).not.toHaveBeenCalled();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it('keeps the selected snapshot when polling replaces the list and returns focus to search', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
    const client = createMockApiClient({ listPanelLogs: vi.fn()
      .mockResolvedValueOnce({ items: [item], total: 1 })
      .mockResolvedValue({ items: [{ ...item, id: 'log:new', code: 'future.event', message: 'New event' }], total: 1 }) });
    show(client);
    await userEvent.click(await screen.findByRole('button', { name: /View details:/ }));
    await act(() => vi.advanceTimersByTimeAsync(5000));
    const dialog = screen.getByRole('dialog');
    expect(dialog.querySelector('pre')).toHaveTextContent('Core start completed');
    expect(within(dialog).getByText('log:one')).toBeVisible();
    expect(within(dialog).queryByText('New event')).not.toBeInTheDocument();
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(await screen.findByText('New event')).toBeVisible();
    await waitFor(() => expect(screen.getByRole('textbox')).toHaveFocus());
  });

  it('preserves long unknown messages and structured error context in details', async () => {
    const message = 'Unrecognized event '.repeat(300);
    const error = { retry: false, attempts: [0, 1] };
    show(createMockApiClient({ listPanelLogs: vi.fn().mockResolvedValue({ items: [{
      ...item, code: 'future.event', message, metadata: { error_code: 'future_error', error },
    }], total: 1 }) }));
    await userEvent.click(await screen.findByRole('button', { name: /View details:/ }));
    const dialog = screen.getByRole('dialog');
    await waitFor(() => expect(within(dialog).getByRole('heading', { name: 'Log details' })).toHaveFocus());
    expect(JSON.parse(dialog.querySelector('pre')!.textContent!)).toHaveProperty('message', message);
    expect(dialog.querySelector('.panel-log-value')?.textContent).toBe(message);
    expect(within(dialog).getByRole('region', { name: 'Event context' }).textContent).toContain(JSON.stringify(error, null, 2));
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  it('handles missing historical context and clipboard rejection without closing the dialog', async () => {
    const user = userEvent.setup();
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('denied'));
    const notify = vi.spyOn(toast, 'add');
    show();
    await user.click(await screen.findByRole('button', { name: /View details:/ }));
    expect(screen.getByText('No additional context was recorded.')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Copy log' }));
    await waitFor(() => expect(notify).toHaveBeenCalledWith(expect.objectContaining({ title: 'Could not copy the log', type: 'error' })));
    expect(screen.getByRole('dialog')).toBeVisible();
  });

  it('keeps failure context in details and preserves the original message', async () => {
    await i18n.changeLanguage('zh-CN');
    show(createMockApiClient({ listPanelLogs: vi.fn().mockResolvedValue({ items: [{
      ...item, code: 'runtime.start.failed', message: 'Core start failed', level: 'error',
      metadata: { error_code: 'core_not_enabled', error: 'core is not enabled', file: 'not-the-reason' },
    }], total: 1 }) }));
    expect(await screen.findByRole('cell', { name: '核心启动操作失败' })).toBeVisible();
    expect(screen.queryByText('请先启用核心版本。')).not.toBeInTheDocument();
    expect(screen.queryByText('not-the-reason')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /查看详情:/ }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('请先启用核心版本。')).toBeVisible();
    expect(within(dialog).queryByRole('region', { name: '原始消息' })).not.toBeInTheDocument();
    expect(dialog.querySelector('pre')).toHaveTextContent('Core start failed');
    expect(within(dialog).getByText('core_not_enabled')).toBeVisible();
  });

  it('searches translated names on the server and resets numbered pagination', async () => {
    const client = show(createMockApiClient({
      listPanelLogs: vi.fn().mockResolvedValue({ items: [item], total: 21 }),
    }));
    await screen.findByText('Core start operation completed');
    await userEvent.click(screen.getByRole('button', { name: 'Next page' }));
    await waitFor(() => expect(client.listPanelLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 10 }), expect.any(AbortSignal),
    ));
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '核心启动' } });
    await waitFor(() => expect(client.listPanelLogs).toHaveBeenLastCalledWith(expect.objectContaining({
      search: '核心启动', searchCodes: expect.arrayContaining(['runtime.start.completed', 'runtime.start.failed']), offset: 0,
    }), expect.any(AbortSignal)));
    expect(screen.getByRole('spinbutton')).toHaveValue(1);
    expect(matchingEventCodes(' CORE START ')).toContain('runtime.start.completed');
    expect(matchingEventCodes('核心已启动')).toEqual(['start_succeeded']);
    expect(matchingEventCodes('unrecognized words')).toBeUndefined();
    expect(matchingEventCodes('')).toBeUndefined();
  });
});
