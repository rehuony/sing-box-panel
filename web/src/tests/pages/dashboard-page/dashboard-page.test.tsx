import { MemoryRouter } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { ApiClientProvider } from '@/api/api-client-context';
import { DashboardPage } from '@/pages/dashboard-page/dashboard-page';
import { createMockApiClient, testMetrics, testMetricsHistory } from '@/tests/api/mock-api-client';

beforeAll(() => vi.stubGlobal('ResizeObserver', class {
  observe() {} disconnect() {}
}));
afterEach(() => vi.useRealTimers());
afterAll(() => vi.unstubAllGlobals());

function show(client = createMockApiClient()) {
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <DashboardPage />
      </ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}
describe('dashboard evidence', () => {
  it('paginates runtime history within the API limit of 200', async () => {
    const client = createMockApiClient({ getRuntimeHistory: vi.fn()
      .mockResolvedValueOnce({ items: [], next: { id: 'transition-1', occurred_at: '2026-09-20T00:00:00Z' } })
      .mockResolvedValue({ items: [] }) });
    show(client);
    await waitFor(() => expect(client.getRuntimeHistory).toHaveBeenCalledTimes(2));
    expect(client.getRuntimeHistory).toHaveBeenNthCalledWith(
      1, expect.objectContaining({ limit: 200 }), expect.any(AbortSignal),
    );
    expect(client.getRuntimeHistory).toHaveBeenNthCalledWith(2, expect.objectContaining({
      limit: 200, beforeID: 'transition-1', beforeTime: '2026-09-20T00:00:00Z',
    }), expect.any(AbortSignal));
  });

  it('shows separate traffic and connection charts with a fixed one-hour connection window', async () => {
    const client = show();
    expect(screen.getAllByRole('figure')).toHaveLength(2);
    expect(screen.queryByRole('button', { name: 'Refresh' })).not.toBeInTheDocument();
    await waitFor(() =>
      expect(client.getMetricsHistory).toHaveBeenCalledWith(
        expect.objectContaining({ bucketSeconds: 60 }),
        expect.any(AbortSignal),
      ),
    );
    expect(screen.getByRole('tab', { name: '1h', selected: true })).toBeVisible();
    await userEvent.click(screen.getByRole('tab', { name: '24h' }));
    expect(screen.getByRole('tab', { name: '24h', selected: true })).toBeVisible();
    expect(screen.getByRole('tabpanel', { name: '24h' })).toBeVisible();
    await waitFor(() =>
      expect(client.getMetricsHistory).toHaveBeenCalledWith(
        expect.objectContaining({ bucketSeconds: 300 }),
        expect.any(AbortSignal),
      ),
    );
    const calls = vi.mocked(client.getMetricsHistory).mock.calls;
    expect(
      calls.some(
        ([filter]) =>
          Date.parse(filter.to) - Date.parse(filter.from) === 3_600_000
          && filter.bucketSeconds === 60,
      ),
    ).toBe(true);
    expect(
      calls.some(
        ([filter]) =>
          Date.parse(filter.to) - Date.parse(filter.from) === 86_400_000
          && filter.bucketSeconds === 300,
      ),
    ).toBe(true);
    await userEvent.keyboard('{ArrowLeft}{Enter}');
    expect(screen.getByRole('tab', { name: '1h', selected: true })).toHaveFocus();
    expect(screen.getByRole('tabpanel', { name: '1h' })).toBeVisible();
    expect(screen.getAllByRole('figure')).toHaveLength(2);
  });
  it('never replaces missing host readings with sample values or core process memory', async () => {
    show();
    await waitFor(() => expect(screen.getByText('Host memory')).toBeVisible());
    const card = screen.getByText('Host memory').closest('section')!;
    expect(card.querySelector('strong')).toHaveTextContent('—');
  });
  it('advances the window without clearing the last chart while new samples load', async () => {
    vi.useFakeTimers({ now: new Date('2026-08-26T07:40:00Z') });
    const client = createMockApiClient();
    await act(async () => {
      show(client);
    });
    const chart = screen.getByRole('figure', { name: 'Traffic' });
    fireEvent.keyDown(chart, { key: 'ArrowLeft' });
    const previousTooltip = within(chart).getByRole('status').textContent;
    let resolveHistory!: (history: typeof testMetricsHistory) => void;
    client.getMetricsHistory.mockImplementationOnce(() => new Promise((resolve) => {
      resolveHistory = resolve;
    }));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });

    expect(client.getMetricsHistory).toHaveBeenLastCalledWith({
      from: '2026-08-26T06:40:30.000Z',
      to: '2026-08-26T07:40:30.000Z',
      bucketSeconds: 60,
    }, expect.any(AbortSignal));
    expect(within(chart).getByRole('status')).toHaveTextContent(previousTooltip!);
    await act(async () => {
      resolveHistory(testMetricsHistory);
    });
  });
  it('uses actual host memory and not the core memory sample', async () => {
    show(
      createMockApiClient({
        getMetrics: vi
          .fn()
          .mockResolvedValue({
            ...testMetrics,
            host: {
              sampled_at: new Date().toISOString(),
              cpu_count: 4,
              cpu_percent: 12.8,
              load_one: 0.64,
              memory_total: 1000,
              memory_used: 500,
              disk_total: 2000,
              disk_used: 500,
            },
          }),
      }),
    );
    await screen.findByText('50.0%');
    expect(screen.getByText('12.8%')).toBeVisible();
  });
  it('renders exactly 48 unknown segments when runtime history is unavailable', async () => {
    const addToast = vi.spyOn(toast, 'add');
    show(
      createMockApiClient({
        getRuntimeHistory: vi.fn().mockRejectedValue(new Error('unavailable')),
      }),
    );
    await waitFor(() => expect(addToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', description: 'unavailable' })));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    addToast.mockRestore();
    const segments = document.querySelectorAll('.runtime-timeline > span');
    expect(segments).toHaveLength(48);
    expect([...segments].every((segment) => segment.getAttribute('data-state') === 'unknown')).toBe(
      true,
    );
  });
});
