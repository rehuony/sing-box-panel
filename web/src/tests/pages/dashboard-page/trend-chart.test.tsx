import UPlot from 'uplot';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';

import '@/i18n';
import { TrendChart } from '@/pages/dashboard-page/trend-chart';
import { testMetricsHistory } from '@/tests/api/mock-api-client';
import { trendChartData } from '@/pages/dashboard-page/trend-chart-data';

vi.mock('uplot', () => ({
  default: vi.fn(class {
    options: UPlot.Options;
    data: UPlot.AlignedData = [[], [], []];
    cursor = { idx: null as number | null, left: -10, top: -10 };
    over = document.createElement('div');
    constructor(options: UPlot.Options) {
      this.options = options;
      Object.defineProperties(this.over, {
        offsetLeft: { value: 32 },
        offsetTop: { value: 8 },
      });
    }

    batch = (update: () => void) => update();
    setData = vi.fn((data: UPlot.AlignedData) => {
      this.data = data;
    });

    setScale = vi.fn();
    setSize = vi.fn();
    destroy = vi.fn();
    valToPos = vi.fn((value: number, scale: string) =>
      scale === 'x' ? this.data[0].indexOf(value) * 250 : 100);

    setCursor = vi.fn(({ left, top }: { left: number; top: number }) => {
      this.cursor = { left, top, idx: left < 0 || top < 0 ? null : Math.round(left / 250) };
      this.options.hooks?.setCursor?.forEach(hook => hook?.(this as unknown as UPlot));
    });
  }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(640);
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(240);
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(160);
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(70);
});

describe('chart details', () => {
  function moveCursor(index: number, left: number, top: number) {
    const plot = vi.mocked(UPlot).mock.results[0]!.value as UPlot;
    act(() => {
      Object.assign(plot.cursor, { idx: index, left, top });
      vi.mocked(UPlot).mock.calls[0]![0].hooks?.setCursor?.forEach(hook => hook?.(plot));
    });
  }

  it('shows each transfer rate with its unit and preserves missing values', async () => {
    const history = {
      ...testMetricsHistory,
      buckets: [
        { ...testMetricsHistory.buckets[0]!, download_bytes: 307_200, upload_bytes: 614_400 },
        testMetricsHistory.buckets[1]!,
      ],
    };
    render(<TrendChart chart={trendChartData(history, 'traffic', [])} kind='traffic' />);
    moveCursor(0, 100, 40);
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Download1.0 KB/sUpload2.0 KB/s'));

    moveCursor(1, 200, 40);
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Download— KB/sUpload— KB/s'));
  });

  it('anchors keyboard details to the sample, respects bounds, and dismisses on Escape or blur', async () => {
    render(<TrendChart chart={trendChartData(testMetricsHistory, 'connections', [])} kind='connections' />);
    const chart = screen.getByRole('figure');
    const tooltip = screen.getByRole('status');
    const plot = vi.mocked(UPlot).mock.results[0]!.value as UPlot;
    fireEvent.keyDown(chart, { key: 'ArrowRight' });
    await waitFor(() => expect(tooltip).toHaveTextContent('Connections12.5 count'));
    expect(plot.valToPos).toHaveBeenCalledWith(Date.parse(testMetricsHistory.buckets[0]!.to), 'x');

    fireEvent.keyDown(chart, { key: 'ArrowRight' });
    fireEvent.keyDown(chart, { key: 'ArrowRight' });
    await waitFor(() => expect(tooltip).toHaveTextContent('Connections— count'));
    fireEvent.keyDown(chart, { key: 'ArrowLeft' });
    await waitFor(() => expect(tooltip).toHaveTextContent('Connections12.5 count'));

    fireEvent.keyDown(chart, { key: 'Escape' });
    expect(tooltip).not.toHaveClass('trend-chart__tooltip');
    fireEvent.keyDown(chart, { key: 'ArrowRight' });
    fireEvent.blur(chart);
    expect(tooltip).not.toHaveClass('trend-chart__tooltip');
  });

  it('updates plotted values and an open tooltip together when live traffic changes units', async () => {
    const chart = (downloadBytesPerSecond: number) => trendChartData(null, 'traffic', [{
      at: 1000, downloadBytesPerSecond, uploadBytesPerSecond: 512, connections: 2,
    }]);
    const { rerender } = render(<TrendChart chart={chart(1024)} kind='traffic' />);
    moveCursor(0, 100, 40);
    const tooltip = screen.getByRole('status');
    await waitFor(() => expect(tooltip).toHaveTextContent('Download1.0 KB/sUpload0.50 KB/s'));

    rerender(<TrendChart chart={chart(2 * 1024 ** 2)} kind='traffic' />);
    expect(tooltip).toHaveTextContent('Download2.0 MB/sUpload0.000488 MB/s');
    const plot = vi.mocked(UPlot).mock.results[0]!.value as UPlot;
    expect(plot.setData).toHaveBeenLastCalledWith([[1000], [2], [512 / 1024 ** 2]]);

    rerender(<TrendChart chart={chart(512)} kind='traffic' />);
    expect(tooltip).toHaveTextContent('Download512 B/sUpload512 B/s');
    expect(UPlot).toHaveBeenCalledTimes(1);
  });
});
afterEach(() => vi.restoreAllMocks());

describe('rolling trend charts', () => {
  it.each(['traffic', 'connections'] as const)(
    'updates %s in place, removes expired samples, and preserves missing data',
    (kind) => {
      const chart = trendChartData(testMetricsHistory, kind, []);
      const { rerender, unmount } = render(<TrendChart chart={chart} kind={kind} />);
      const plot = vi.mocked(UPlot).mock.results[0]!.value as UPlot;
      const next = {
        ...testMetricsHistory,
        from: '2026-08-26T07:35:00Z',
        to: '2026-08-26T07:45:00Z',
        buckets: [
          testMetricsHistory.buckets[1]!,
          {
            ...testMetricsHistory.buckets[0]!,
            from: '2026-08-26T07:40:00Z',
            to: '2026-08-26T07:45:00Z',
            download_bytes: 307_200,
            upload_bytes: 614_400,
          },
        ],
      };

      rerender(<TrendChart chart={trendChartData(next, kind, [])} kind={kind} />);

      expect(UPlot).toHaveBeenCalledTimes(1);
      expect(plot.destroy).not.toHaveBeenCalled();
      const times = [Date.parse(next.buckets[0]!.to), Date.parse(next.to)];
      expect(plot.setData).toHaveBeenLastCalledWith(
        kind === 'traffic' ? [times, [null, 1], [null, 2]] : [times, [null, 12.5]],
      );
      expect(plot.setScale).toHaveBeenLastCalledWith('x', {
        min: Date.parse(next.from),
        max: Date.parse(next.to),
      });

      rerender(<TrendChart chart={trendChartData(null, kind, [])} kind={kind} />);
      expect(plot.setData).toHaveBeenLastCalledWith(kind === 'traffic' ? [[], [], []] : [[], []]);
      unmount();
      expect(plot.destroy).toHaveBeenCalledTimes(1);
    },
  );
});
