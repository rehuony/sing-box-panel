import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import { createMemoryRouter, RouterProvider, useLocation } from 'react-router-dom';

import type { ApiClient, LogEntry, LogFilter, LogStreamFilter, MetricsSnapshot } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { ObservabilityPage } from '@/pages/observability-page/observability-page';
import {
  createMockApiClient,
  testLogEntry,
  testMetrics,
  testTrafficPeriod,
} from '@/tests/api/mock-api-client';

function renderObservability(client: ApiClient, initialEntry = '/observability') {
  const router = createMemoryRouter([
    {
      path: '/observability',
      element: (
        <ApiClientProvider client={client}>
          <ObservabilityPage />
          <LocationProbe />
        </ApiClientProvider>
      ),
    },
  ], { initialEntries: [initialEntry] });
  render(<RouterProvider router={router} />);
  return router;
}

function LocationProbe() {
  const location = useLocation();
  return <output data-testid='location-search'>{location.search}</output>;
}

describe('observabilityPage', () => {
  it('separates summary, traffic, and logs into compact tabs', async () => {
    const client = createMockApiClient();
    renderObservability(client);

    expect(screen.getByRole('tab', { name: 'Summary' })).toHaveAttribute('data-active');
    expect(screen.queryByText('The exact core process passed its health check.')).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Traffic periods' })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('tab', { name: 'Traffic' }));
    expect(screen.getByRole('heading', { name: 'Traffic periods' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Evidence summary' })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('tab', { name: 'Logs' }));
    expect(await screen.findByRole('heading', { name: 'Event log' })).toBeInTheDocument();
  });

  it('distinguishes unavailable collector evidence from zero traffic', async () => {
    const unavailableMetrics: MetricsSnapshot = {
      available: false,
      traffic_available: false,
      collected_at: '2026-08-26T07:34:00Z',
      quota_exceeded: false,
      reason_code: 'not_applied',
    };
    const client = createMockApiClient({
      getMetrics: async () => unavailableMetrics,
      getTrafficStatus: async () => unavailableMetrics,
    });
    renderObservability(client);

    await screen.findByText('No bundle is applied');
    const summary = screen.getByRole('heading', { name: 'Evidence summary' }).closest('section');
    expect(summary).not.toBeNull();
    expect(within(summary as HTMLElement).getAllByText('Not reported')).toHaveLength(2);
    expect(within(summary as HTMLElement).getByText('Not evaluated')).toBeInTheDocument();
    expect(within(summary as HTMLElement).queryByText(/^0 B$/)).not.toBeInTheDocument();
  });

  it('clears stale summary evidence when a refresh fails', async () => {
    const getMetrics = vi.fn()
      .mockResolvedValueOnce(testMetrics)
      .mockRejectedValueOnce(new Error('collector offline'));
    const getTrafficStatus = vi.fn()
      .mockResolvedValueOnce(testMetrics)
      .mockRejectedValueOnce(new Error('traffic collector offline'));
    const client = createMockApiClient({ getMetrics, getTrafficStatus });
    renderObservability(client);
    const user = userEvent.setup();

    const summary = screen.getByRole('heading', { name: 'Evidence summary' }).closest('section');
    expect(summary).not.toBeNull();
    expect(await within(summary as HTMLElement).findByText('Available')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Refresh' }));

    expect(await within(summary as HTMLElement).findByText('Snapshot unavailable')).toBeInTheDocument();
    expect(within(summary as HTMLElement).queryByText('Available')).not.toBeInTheDocument();
    expect(within(summary as HTMLElement).getAllByText('Not reported')).toHaveLength(2);
  });

  it('filters and paginates persisted traffic with a paired stable cursor', async () => {
    const listTrafficPeriods = vi.fn()
      .mockResolvedValueOnce({
        items: [testTrafficPeriod],
        next: { period_start: testTrafficPeriod.period_start, id: testTrafficPeriod.id },
      })
      .mockResolvedValueOnce({
        items: [testTrafficPeriod],
        next: { period_start: testTrafficPeriod.period_start, id: testTrafficPeriod.id },
      })
      .mockResolvedValue({ items: [] });
    const client = createMockApiClient({ listTrafficPeriods });
    renderObservability(client);
    const user = userEvent.setup();

    await user.click(screen.getByRole('tab', { name: 'Traffic' }));
    await screen.findByRole('button', { name: /2,048 B/ });
    await user.type(screen.getByPlaceholderText('All bundles'), 'bundle_18');
    await user.click(screen.getByRole('button', { name: 'Apply filters' }));

    await waitFor(() => expect(listTrafficPeriods).toHaveBeenCalledWith(
      expect.objectContaining({ activationBundleID: 'bundle_18', limit: 30 }),
      expect.any(AbortSignal),
    ));

    await user.click(screen.getByRole('button', { name: 'Load older' }));
    expect(listTrafficPeriods).toHaveBeenCalledWith(
      expect.objectContaining({
        beforeID: testTrafficPeriod.id,
        beforeTime: testTrafficPeriod.period_start,
      }),
      undefined,
    );
  });

  it('uses traffic URL filters as applied state across external navigation and history', async () => {
    const listTrafficPeriods = vi.fn().mockResolvedValue({ items: [testTrafficPeriod] });
    const client = createMockApiClient({ listTrafficPeriods });
    const firstLocation = '/observability?tab=traffic&activation_bundle_id=bundle_first&from=2026-08-30T01%3A00%3A00Z&to=2026-08-30T02%3A00%3A00Z';
    const router = renderObservability(client, firstLocation);
    const user = userEvent.setup();

    const bundle = screen.getByPlaceholderText('All bundles');
    expect(bundle).toHaveValue('bundle_first');
    await waitFor(() => expect(listTrafficPeriods).toHaveBeenLastCalledWith(
      expect.objectContaining({
        activationBundleID: 'bundle_first',
        from: '2026-08-30T01:00:00Z',
        to: '2026-08-30T02:00:00Z',
      }),
      expect.any(AbortSignal),
    ));

    await user.clear(bundle);
    await user.type(bundle, 'unapplied_draft');
    await act(async () => {
      await router.navigate('/observability?tab=traffic&activation_bundle_id=bundle_second&from=2026-08-30T03%3A00%3A00Z&to=2026-08-30T04%3A00%3A00Z');
    });
    expect(bundle).toHaveValue('bundle_second');
    await waitFor(() => expect(listTrafficPeriods).toHaveBeenLastCalledWith(
      expect.objectContaining({
        activationBundleID: 'bundle_second',
        from: '2026-08-30T03:00:00Z',
        to: '2026-08-30T04:00:00Z',
      }),
      expect.any(AbortSignal),
    ));

    await act(async () => {
      await router.navigate(-1);
    });
    expect(bundle).toHaveValue('bundle_first');
    await waitFor(() => expect(listTrafficPeriods).toHaveBeenLastCalledWith(
      expect.objectContaining({ activationBundleID: 'bundle_first' }),
      expect.any(AbortSignal),
    ));
  });

  it('opens traffic detail in a controlled sheet without extending the surrounding list', async () => {
    const client = createMockApiClient();
    renderObservability(client);
    const user = userEvent.setup();
    await user.click(screen.getByRole('tab', { name: 'Traffic' }));
    const trafficRow = await screen.findByRole('button', { name: /2,048 B/ });
    await user.click(trafficRow);

    expect(client.getTrafficPeriod).toHaveBeenCalledWith(testTrafficPeriod.id);
    expect(within(screen.getByRole('dialog'))
      .getByRole('heading', { name: testTrafficPeriod.id })).toBeInTheDocument();
    expect(screen.queryByRole('complementary', { name: 'Traffic period detail' })).not.toBeInTheDocument();
  });

  it('opens and dismisses log inspection in a sheet', async () => {
    const client = createMockApiClient();
    renderObservability(client, '/observability?tab=logs');
    const user = userEvent.setup();
    const logRow = await screen.findByRole('button', { name: /The exact core process passed its health check/ });

    await user.click(logRow);

    expect(client.getLog).toHaveBeenCalledWith(testLogEntry.id);
    const detail = screen.getByRole('dialog');
    expect(within(detail).getByRole('heading', { name: testLogEntry.code })).toBeInTheDocument();
    expect(within(detail).getByText(testLogEntry.id)).toBeInTheDocument();
    expect(screen.queryByRole('complementary', { name: 'Log detail' })).not.toBeInTheDocument();

    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  it('applies code and time filters to both log pages and the live stream', async () => {
    const streamedEntry: LogEntry = {
      ...testLogEntry,
      id: 'log_streamed_warning',
      code: 'runtime.streamed',
      level: 'warn',
      message: 'A filtered live event arrived.',
    };
    const streamLogs = vi.fn(async function* (
      _filter?: LogStreamFilter,
      signal?: AbortSignal,
    ) {
      yield { id: 'stream_cursor_1', entry: streamedEntry };
      await new Promise<void>((resolve) => {
        if (signal?.aborted) resolve();
        else signal?.addEventListener('abort', () => resolve(), { once: true });
      });
    });
    const client = createMockApiClient({ streamLogs });
    renderObservability(
      client,
      '/observability?tab=logs&from=2026-08-30T01%3A00%3A00Z&to=2026-08-30T02%3A00%3A00Z',
    );
    const user = userEvent.setup();
    await screen.findByRole('heading', { name: 'Event log' });
    await user.type(screen.getByLabelText('Code'), 'runtime.ready');
    await user.click(screen.getByRole('button', { name: 'Start live stream' }));

    expect(await screen.findByRole('button', { name: /A filtered live event arrived/ })).toBeInTheDocument();
    expect(streamLogs).toHaveBeenCalledWith(
      expect.objectContaining({
        code: 'runtime.ready',
        since: '2026-08-30T01:00:00Z',
      }),
      expect.any(AbortSignal),
    );
    expect(client.listLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({
        code: 'runtime.ready',
        since: '2026-08-30T01:00:00Z',
        until: '2026-08-30T02:00:00Z',
      }),
      expect.any(AbortSignal),
    );
    await waitFor(() => expect(screen.getByTestId('location-search')).toHaveTextContent('code=runtime.ready'));
  });

  it('aborts a live stream when external URL filters change and ignores its late events', async () => {
    const lateEntry: LogEntry = {
      ...testLogEntry,
      id: 'log_from_stale_stream',
      message: 'Stale live event must not appear.',
    };
    let releaseLateEntry: ((entry: LogEntry) => void) | undefined;
    const pendingEntry = new Promise<LogEntry>((resolve) => {
      releaseLateEntry = resolve;
    });
    const streamLogs = vi.fn(async function* (
      _filter?: LogStreamFilter,
      _signal?: AbortSignal,
    ) {
      const entry = await pendingEntry;
      yield { id: 'stale_stream_cursor', entry };
    });
    const client = createMockApiClient({ streamLogs });
    const router = renderObservability(client, '/observability?tab=logs&code=first');
    const user = userEvent.setup();

    await user.click(await screen.findByRole('button', { name: 'Start live stream' }));
    await waitFor(() => expect(streamLogs).toHaveBeenCalledWith(
      expect.objectContaining({ code: 'first' }),
      expect.any(AbortSignal),
    ));
    const signal = streamLogs.mock.calls[0]?.[1] as AbortSignal;

    await act(async () => {
      await router.navigate('/observability?tab=logs&code=second');
    });
    expect(signal.aborted).toBe(true);
    expect(await screen.findByRole('button', { name: 'Start live stream' })).toBeInTheDocument();
    await waitFor(() => expect(client.listLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ code: 'second' }),
      expect.any(AbortSignal),
    ));

    await act(async () => {
      releaseLateEntry?.(lateEntry);
      await Promise.resolve();
    });
    expect(screen.queryByText('Stale live event must not appear.')).not.toBeInTheDocument();

    await act(async () => {
      await router.navigate(-1);
    });
    expect(await screen.findByRole('button', { name: 'Start live stream' })).toBeInTheDocument();
    expect(streamLogs).toHaveBeenCalledTimes(1);
  });

  it('follows external search changes and browser history without restoring stale log filters', async () => {
    const listLogs = vi.fn().mockResolvedValue({ items: [testLogEntry] });
    const client = createMockApiClient({ listLogs });
    const router = renderObservability(client, '/observability?tab=summary');

    await act(async () => {
      await router.navigate('/observability?tab=logs&source=core&level=warn&code=first&from=2026-08-30T01%3A00%3A00Z&to=2026-08-30T02%3A00%3A00Z');
    });

    expect(await screen.findByRole('heading', { name: 'Event log' })).toBeInTheDocument();
    expect(screen.getByLabelText('Source')).toHaveValue('core');
    expect(screen.getByLabelText('Level')).toHaveValue('warn');
    expect(screen.getByLabelText('Code')).toHaveValue('first');
    expect(screen.getByLabelText('Log start time')).toHaveValue('2026-08-30T01:00:00Z');
    expect(screen.getByLabelText('Log end time')).toHaveValue('2026-08-30T02:00:00Z');

    await act(async () => {
      await router.navigate('/observability?tab=logs&source=panel&level=error&code=second&from=2026-08-30T03%3A00%3A00Z&to=2026-08-30T04%3A00%3A00Z');
    });

    await waitFor(() => {
      expect(screen.getByLabelText('Source')).toHaveValue('panel');
      expect(screen.getByLabelText('Level')).toHaveValue('error');
      expect(screen.getByLabelText('Code')).toHaveValue('second');
      expect(screen.getByLabelText('Log start time')).toHaveValue('2026-08-30T03:00:00Z');
      expect(screen.getByLabelText('Log end time')).toHaveValue('2026-08-30T04:00:00Z');
    });
    await waitFor(() => expect(listLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({
        code: 'second',
        level: 'error',
        since: '2026-08-30T03:00:00Z',
        source: 'panel',
        until: '2026-08-30T04:00:00Z',
      }),
      expect.any(AbortSignal),
    ));

    await act(async () => {
      await router.navigate(-1);
    });
    await waitFor(() => {
      expect(screen.getByLabelText('Source')).toHaveValue('core');
      expect(screen.getByLabelText('Level')).toHaveValue('warn');
      expect(screen.getByLabelText('Code')).toHaveValue('first');
    });

    await act(async () => {
      await router.navigate(-1);
    });
    expect(screen.getByRole('tab', { name: 'Summary' })).toHaveAttribute('data-active');
    expect(screen.queryByRole('heading', { name: 'Event log' })).not.toBeInTheDocument();

    await act(async () => {
      await router.navigate(1);
    });
    await waitFor(() => {
      expect(screen.getByLabelText('Source')).toHaveValue('core');
      expect(screen.getByLabelText('Code')).toHaveValue('first');
    });

    await act(async () => {
      await router.navigate(1);
    });
    await waitFor(() => {
      expect(screen.getByLabelText('Source')).toHaveValue('panel');
      expect(screen.getByLabelText('Level')).toHaveValue('error');
      expect(screen.getByLabelText('Code')).toHaveValue('second');
    });
  });

  it('clears only the exact source scope and blocks unsupported clear filters', async () => {
    const client = createMockApiClient();
    renderObservability(client, '/observability?tab=logs');
    const user = userEvent.setup();
    await screen.findByRole('heading', { name: 'Event log' });

    await user.selectOptions(screen.getByLabelText('Source'), 'core');
    await user.type(screen.getByLabelText('Code'), 'runtime.ready');
    const clearButton = screen.getByRole('button', { name: 'Clear filtered events' });
    expect(clearButton).toBeDisabled();
    expect(screen.getByText(/server can safely scope this action only by source/i)).toBeInTheDocument();

    await user.clear(screen.getByLabelText('Code'));
    await user.type(screen.getByLabelText('Log end time'), '2026-08-30T02:00:00Z');
    expect(clearButton).toBeEnabled();
    await user.click(clearButton);
    expect(screen.getByText(/Clear every persisted event in Core/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Confirm clear' }));

    await waitFor(() => expect(client.clearLogs).toHaveBeenCalledWith({
      before: '2026-08-30T02:00:00.000Z',
      source: 'core',
    }));
  });

  it('ignores a late log response from an older debounced filter', async () => {
    let resolveFirst: ((value: { items: LogEntry[] }) => void) | undefined;
    let resolveSecond: ((value: { items: LogEntry[] }) => void) | undefined;
    const firstRequest = new Promise<{ items: LogEntry[] }>((resolve) => {
      resolveFirst = resolve;
    });
    const secondRequest = new Promise<{ items: LogEntry[] }>((resolve) => {
      resolveSecond = resolve;
    });
    const firstEntry = { ...testLogEntry, id: 'log_first', message: 'Older filter result.' };
    const secondEntry = { ...testLogEntry, id: 'log_second', message: 'Latest filter result.' };
    const listLogs = vi.fn(async (filter?: LogFilter) => {
      if (filter?.code === 'first') return firstRequest;
      if (filter?.code === 'second') return secondRequest;
      return { items: [testLogEntry] };
    });
    const client = createMockApiClient({ listLogs });
    renderObservability(client, '/observability?tab=logs');
    const user = userEvent.setup();
    const codeInput = screen.getByLabelText('Code');
    await screen.findByRole('button', { name: /The exact core process passed its health check/ });

    await user.type(codeInput, 'first');
    await waitFor(() => expect(listLogs).toHaveBeenCalledWith(
      expect.objectContaining({ code: 'first' }),
      expect.any(AbortSignal),
    ));
    await user.clear(codeInput);
    await user.type(codeInput, 'second');
    await waitFor(() => expect(listLogs).toHaveBeenCalledWith(
      expect.objectContaining({ code: 'second' }),
      expect.any(AbortSignal),
    ));

    resolveSecond?.({ items: [secondEntry] });
    expect(await screen.findByRole('button', { name: /Latest filter result/ })).toBeInTheDocument();
    resolveFirst?.({ items: [firstEntry] });
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Latest filter result/ })).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /Older filter result/ })).not.toBeInTheDocument();
    });
  });
});
