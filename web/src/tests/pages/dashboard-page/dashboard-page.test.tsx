import { MemoryRouter } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor } from '@testing-library/react';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';

import '@/i18n';
import { ApiClientProvider } from '@/api/api-client-context';
import { DashboardPage } from '@/pages/dashboard-page/dashboard-page';
import { createMockApiClient, testMetrics } from '@/tests/api/mock-api-client';

beforeAll(() => vi.stubGlobal('ResizeObserver', class {
  observe() {} disconnect() {}
}));
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
    await userEvent.click(screen.getByRole('button', { name: '24h' }));
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
  });
  it('never replaces missing host readings with sample values or core process memory', async () => {
    show();
    await waitFor(() => expect(screen.getByText('Host memory')).toBeVisible());
    const card = screen.getByText('Host memory').closest('section')!;
    expect(card.querySelector('strong')).toHaveTextContent('—');
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
    show(
      createMockApiClient({
        getRuntimeHistory: vi.fn().mockRejectedValue(new Error('unavailable')),
      }),
    );
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
    const segments = document.querySelectorAll('.runtime-timeline > span');
    expect(segments).toHaveLength(48);
    expect([...segments].every((segment) => segment.getAttribute('data-state') === 'unknown')).toBe(
      true,
    );
  });
});
