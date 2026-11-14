import { describe, expect, it } from 'vitest';

import type { FleetStatus } from '@/api/api-client';

import { testFleetStatus } from '@/tests/api/mock-api-client';
import { applyFleetFrame } from '@/components/app-shell/use-fleet-stream';

type NodeStatus = FleetStatus['items'][number];

function status(id: string, name: string): NodeStatus {
  const item = testFleetStatus.items[0]!;
  return { ...item, node: { ...item.node, id, name } };
}

const names = (fleet: FleetStatus) => fleet.items.map(item => `${item.node.name}:${item.node.id}`);

describe('applyFleetFrame', () => {
  const held: FleetStatus = { items: [status('node_a', 'alpha'), status('node_b', 'Beta'), status('node_c', 'gamma')] };

  it('replaces the fleet with a reset frame', () => {
    const fleet = applyFleetFrame(held, { reset: true, items: [status('node_d', 'delta')], removed: [] });
    expect(names(fleet)).toEqual(['delta:node_d']);
  });

  it('replaces changed nodes, drops removed ones and keeps the panel order', () => {
    const fleet = applyFleetFrame(held, {
      reset: false,
      // A rename moves a node; a new node takes its place in order, ignoring ASCII case, then by ID.
      items: [status('node_c', 'Aardvark'), status('node_e', 'beta')],
      removed: ['node_a'],
    });
    expect(names(fleet)).toEqual(['Aardvark:node_c', 'Beta:node_b', 'beta:node_e']);
  });
});
