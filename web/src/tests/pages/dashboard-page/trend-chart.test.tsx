import UPlot from 'uplot';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import '@/i18n';
import { TrendChart } from '@/pages/dashboard-page/trend-chart';
import { testMetricsHistory } from '@/tests/api/mock-api-client';

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

  it.each(['traffic', 'connections'] as const)(
    'moves %s details within the same sample and keeps them inside the chart at its edges',
    (kind) => {
      render(<TrendChart history={testMetricsHistory} kind={kind} />);
      moveCursor(0, 100, 40);
      const tooltip = screen.getByRole('status');
      expect(tooltip).toHaveStyle({ left: '146px', top: '62px' });

      moveCursor(0, 200, 60);
      expect(tooltip).toHaveStyle({ left: '246px', top: '82px' });

      moveCursor(0, 600, 200);
      expect(tooltip).toHaveStyle({ left: '458px', top: '124px' });
      expect(tooltip).not.toHaveTextContent(/\d{1,2}:\d{2}/);

      fireEvent.mouseLeave(screen.getByRole('figure'));
      expect(tooltip).toHaveTextContent('Use arrow keys to inspect samples.');
      expect(tooltip).not.toHaveClass('trend-chart__tooltip');
    },
  );

  it('shows each transfer rate with its unit and preserves missing values', () => {
    const history = {
      ...testMetricsHistory,
      buckets: [
        { ...testMetricsHistory.buckets[0]!, download_bytes: 307_200, upload_bytes: 614_400 },
        testMetricsHistory.buckets[1]!,
      ],
    };
    render(<TrendChart history={history} kind='traffic' />);
    moveCursor(0, 100, 40);
    expect(screen.getByRole('status')).toHaveTextContent('Download1.0 KB/sUpload2.0 KB/s');

    moveCursor(1, 200, 40);
    expect(screen.getByRole('status')).toHaveTextContent('Download— KB/sUpload— KB/s');
  });

  it('anchors keyboard details to the sample, respects bounds, and dismisses on Escape or blur', () => {
    render(<TrendChart history={testMetricsHistory} kind='connections' />);
    const chart = screen.getByRole('figure');
    const tooltip = screen.getByRole('status');
    const plot = vi.mocked(UPlot).mock.results[0]!.value as UPlot;
    fireEvent.keyDown(chart, { key: 'ArrowRight' });
    expect(tooltip).toHaveTextContent('Connections12.5 count');
    expect(plot.valToPos).toHaveBeenCalledWith(Date.parse(testMetricsHistory.buckets[0]!.to), 'x');

    fireEvent.keyDown(chart, { key: 'ArrowRight' });
    fireEvent.keyDown(chart, { key: 'ArrowRight' });
    expect(tooltip).toHaveTextContent('Connections— count');
    fireEvent.keyDown(chart, { key: 'ArrowLeft' });
    expect(tooltip).toHaveTextContent('Connections12.5 count');

    fireEvent.keyDown(chart, { key: 'Escape' });
    expect(tooltip).not.toHaveClass('trend-chart__tooltip');
    fireEvent.keyDown(chart, { key: 'ArrowRight' });
    fireEvent.blur(chart);
    expect(tooltip).not.toHaveClass('trend-chart__tooltip');
  });
});
afterEach(() => vi.restoreAllMocks());

describe('rolling trend charts', () => {
  it.each(['traffic', 'connections'] as const)(
    'updates %s in place, removes expired samples, and preserves missing data',
    (kind) => {
      const { rerender, unmount } = render(<TrendChart history={testMetricsHistory} kind={kind} />);
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

      rerender(<TrendChart history={next} kind={kind} />);

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

      rerender(<TrendChart history={null} kind={kind} />);
      expect(plot.setData).toHaveBeenLastCalledWith(kind === 'traffic' ? [[], [], []] : [[], []]);
      unmount();
      expect(plot.destroy).toHaveBeenCalledTimes(1);
    },
  );
});
