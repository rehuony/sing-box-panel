import userEvent from '@testing-library/user-event';
import { toast } from '@shared/components/ui/toast-manager';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { LogEntry } from '@/api/api-client';

import i18n from '@/i18n';
import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { logContext, matchingEventCodes } from '@/components/panel-log';
import { PanelLogsPage } from '@/pages/panel-logs-page/panel-logs-page';

const item: LogEntry = {
  id: 'log:one', time: '2026-09-24T01:16:57.123456Z', source: 'panel', level: 'info',
  code: 'runtime.start.completed', message: 'Core start completed', metadata: {},
};
function show(client = createMockApiClient({ listPanelLogs: vi.fn().mockResolvedValue({ items: [item], total: 1 }) }), entry = '/logs') {
  render(
    <TestRouter initialEntries={[entry]}>
      <ApiClientProvider client={client}><PanelLogsPage /></ApiClientProvider>
    </TestRouter>,
  );
  return client;
}

afterEach(async () => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  await i18n.changeLanguage('en');
});

describe('panel log presentation and details', () => {
  it('localizes known events and keeps unknown ones as recorded', async () => {
    show(createMockApiClient({
      listPanelLogs: vi.fn().mockResolvedValue({ items: [item, { ...item, id: 'log:3', code: 'future.event', message: 'A future event' }], total: 2 }),
    }));
    expect(await screen.findByText('Core start operation completed')).toBeVisible();
    expect(screen.getByText('A future event')).toBeVisible();
  });

  it('shows complete context and raw data, copies the original record and restores focus', async () => {
    const user = userEvent.setup();
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
    const notify = vi.spyOn(toast, 'add');
    const record: LogEntry = { ...item, metadata: {
      duration_ms: 0, enabled: false, missing: null, nested: { values: [0, false, 'safe'] },
      custom_field: '<img src=x onerror=alert(1)>', process_started_at: item.time,
    } };
    show(createMockApiClient({
      listPanelLogs: vi.fn().mockResolvedValue({ items: [record], total: 1 }),
    }));
    // The line names the event and adds what its context records, here its duration.
    const trigger = await screen.findByRole('button', { name: 'Core start operation completed · 0 ms' });
    // Details the keyboard opened return focus to their line.
    trigger.focus();
    await user.keyboard('{Enter}');
    const dialog = screen.getByRole('dialog', { name: 'Log details' });
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
    (await screen.findByRole('button', { name: 'Core start operation completed' })).focus();
    await userEvent.keyboard('{Enter}');
    await act(() => vi.advanceTimersByTimeAsync(5000));
    const dialog = screen.getByRole('dialog');
    expect(dialog.querySelector('pre')).toHaveTextContent('Core start completed');
    expect(within(dialog).getByText('log:one')).toBeVisible();
    expect(within(dialog).queryByText('New event')).not.toBeInTheDocument();
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(await screen.findByText('New event')).toBeVisible();
    await waitFor(() => expect(screen.getByRole('searchbox')).toHaveFocus());
  });

  it('preserves long unknown messages and structured error context in details', async () => {
    const message = 'Unrecognized event '.repeat(300);
    const error = { retry: false, attempts: [0, 1] };
    show(createMockApiClient({ listPanelLogs: vi.fn().mockResolvedValue({ items: [{
      ...item, code: 'future.event', message, metadata: { error_code: 'future_error', error },
    }], total: 1 }) }));
    await userEvent.click(await screen.findByRole('button', { name: /^Unrecognized event/ }));
    const dialog = screen.getByRole('dialog');
    await waitFor(() => expect(within(dialog).getByRole('heading', { name: 'Log details' })).toHaveFocus());
    expect(JSON.parse(dialog.querySelector('pre')!.textContent!)).toHaveProperty('message', message);
    expect(within(dialog).getAllByRole('definition')[0]).toHaveTextContent(message, { normalizeWhitespace: false });
    expect(within(dialog).getByRole('region', { name: 'Event context' }).textContent).toContain(JSON.stringify(error, null, 2));
    await userEvent.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  it('handles missing historical context and clipboard rejection without closing the dialog', async () => {
    const user = userEvent.setup();
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('denied'));
    const notify = vi.spyOn(toast, 'add');
    show();
    await user.click(await screen.findByRole('button', { name: 'Core start operation completed' }));
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
    // The line gives the failure's reason in the interface's language; the details keep the code.
    const trigger = await screen.findByRole('button', { name: '核心启动操作失败 · 请先启用核心版本 · not-the-reason' });
    expect(screen.queryByText('请先启用核心版本。')).not.toBeInTheDocument();
    expect(screen.queryByText('not-the-reason')).not.toBeInTheDocument();
    await userEvent.click(trigger);
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('请先启用核心版本。')).toBeVisible();
    expect(within(dialog).queryByRole('region', { name: '原始消息' })).not.toBeInTheDocument();
    expect(dialog.querySelector('pre')).toHaveTextContent('Core start failed');
    expect(within(dialog).getByText('core_not_enabled')).toBeVisible();
  });

  it('names a line\'s node, a development build and an attempt in its context line', () => {
    const t = i18n.getFixedT('en');
    const names = new Map([['node_edge', 'edge']]);
    const nodeName = (id: string) => names.get(id);
    expect(logContext({ node_id: 'node_edge', version: 'v0.5.5-0.20261127081723-5bbdd30786c1+dirty' }, t, { nodeName }))
      .toBe('edge · v0.5.5 · 5bbdd30+dirty');
    // A node the fleet no longer lists reads by the start of its ID; a node's own log names none.
    expect(logContext({ node_id: 'node_03f56f02342f2b19' }, t, { nodeName })).toBe('03f56f02');
    expect(logContext({ node_id: 'node_edge', attempt: 2, maximum_attempts: 5, duration_ms: 16_960 }, t, {})).toBe('attempt 2 of 5 · 17.0 s');
  });

  it('searches translated names on the server and resets numbered pagination', async () => {
    const client = show(createMockApiClient({
      listPanelLogs: vi.fn().mockResolvedValue({ items: [item], total: 101 }),
    }));
    await screen.findByText('Core start operation completed');
    await userEvent.click(screen.getByRole('button', { name: 'Next page' }));
    await waitFor(() => expect(client.listPanelLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 50 }), expect.any(AbortSignal),
    ));
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: '核心启动' } });
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
