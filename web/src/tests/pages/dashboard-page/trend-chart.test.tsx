import UPlot from 'uplot';
import { render } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import '@/i18n';
import { TrendChart } from '@/pages/dashboard-page/trend-chart';
import { testMetricsHistory } from '@/tests/api/mock-api-client';

vi.mock('uplot', () => ({
  default: vi.fn(class {
    batch = (update: () => void) => update();
    setData = vi.fn();
    setScale = vi.fn();
    setSize = vi.fn();
    destroy = vi.fn();
  }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(640);
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(240);
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
