import { describe, expect, it } from 'vitest';

import type { NodeStatus } from '@/utils/fleet-summary';

import { testMetrics, testNode } from '@/tests/api/mock-api-client';
import { attentionItems, fleetFigures } from '@/utils/fleet-summary';

function status(id: string, node: Partial<NodeStatus['node']>, metrics: Partial<NodeStatus['metrics']> = {}): NodeStatus {
  return { metrics: { ...testMetrics, ...metrics }, node: { ...testNode, id, name: id, ...node } };
}

const stopped = { applied_bundle_id: undefined, desired_running: false, observation_state: 'stopped', target_generation: 1 } as const;
function period(outbound: number, inbound: number) {
  return { ...testMetrics.current_traffic_period, inbound_bytes: inbound, outbound_bytes: outbound };
}

describe('fleet summary', () => {
  it('sums usage but never quotas, and tells a known zero rate from an unknown one', () => {
    const items = [
      status('idle', { runtime: stopped }, { current_traffic_period: period(100, 400), quota_bytes: 1_000 }),
      status('away', { state: 'offline' }, { current_traffic_period: period(50, 50), quota_bytes: 10 }),
    ];
    const figures = fleetFigures(items);
    expect(figures).toMatchObject({
      download: 450, exhausted: 0, offline: 1, online: 1, running: 0, stopped: 1, unknown: 1, upload: 150,
    });
    // The stopped node's rates are a known zero; the offline one adds nothing unknown to them.
    expect(figures.rates).toEqual({ download: 0, upload: 0 });
    expect(fleetFigures([items[1]!]).rates).toEqual({ download: null, upload: null });
  });

  it('lists what needs a look, most severe first', () => {
    const items = [
      status('pending', { state: 'pending' }),
      status('near', { runtime: stopped }, { current_traffic_period: period(450, 500), quota_bytes: 1_000 }),
      status('gone', { state: 'offline' }),
      status('down', { runtime: { ...stopped, desired_running: true } }),
    ];
    expect(attentionItems(items).map(item => [item.item.node.id, item.kind])).toEqual([
      ['gone', 'offline'],
      ['down', 'coreDown'],
      ['near', 'quotaNear'],
      ['pending', 'pending'],
    ]);
  });
});
