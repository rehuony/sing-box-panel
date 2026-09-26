import { describe, expect, it } from 'vitest';

import type { SubscriptionNodeSummary } from '@/api/api-client';

import { candidateOrder } from '@/pages/subscriptions-page/channel-node-order';
import { filterSourceNodes, orderSourceNodes, reorderVisibleNodes } from '@/pages/subscriptions-page/subscription-node-order';

function node(id: string, extra: Partial<SubscriptionNodeSummary> = {}): SubscriptionNodeSummary {
  return { id, name: id, key: id, tag: id, source_id: 'source', source_name: 'Remote', origin: 'source',
    type: 'socks', server: 'proxy.example', port: 1080, hidden: false, available: true,
    tls: false, reality: false, revision: 1, visibility_revision: 0, ...extra };
}

describe('candidate ordering', () => {
  it('keeps filtered-out slots when moving visible cards', () => {
    const order = ['node:a', 'builtin:direct', 'node:b', 'builtin:reject'];
    expect(reorderVisibleNodes(order, ['node:a', 'node:b'], 'node:b', 'node:a'))
      .toEqual(['node:b', 'builtin:direct', 'node:a', 'builtin:reject']);
    expect(reorderVisibleNodes(order, ['node:a'], 'unknown', 'node:a')).toBe(order);
  });
  it('removes deleted candidates and appends added ones without losing saved mixed order', () => {
    expect(candidateOrder({
      node_ids: ['b', 'c'], builtin_nodes: ['direct'],
      candidate_order: ['node:b', 'builtin:reject', 'node:a', 'builtin:direct'],
    })).toEqual(['node:b', 'builtin:direct', 'node:c']);
  });
});

describe('source node lists', () => {
  it('reconciles saved order with removed, added and duplicated IDs without mutating the catalog', () => {
    const nodes = [node('a'), node('b'), node('new')];
    expect(orderSourceNodes(nodes, ['b', 'removed', 'b', 'a'])).toEqual([nodes[1], nodes[0], nodes[2]]);
    expect(nodes.map(node => node.id)).toEqual(['a', 'b', 'new']);
  });
  it.each(['EDGE', 'remote', 'SOCKS'])('matches names, sources and protocols case-insensitively: %s', search => {
    const nodes = [node('Edge'), node('other', { source_name: 'Local', type: 'http' })];
    expect(filterSourceNodes(nodes, search, false)).toEqual([nodes[0]]);
  });
  it('excludes unavailable and hidden candidates before pagination while keeping them in source management', () => {
    const nodes = [node('hidden', { hidden: true }), node('unavailable', { available: false }), node('available')];
    expect(filterSourceNodes(nodes, '', true)).toEqual([nodes[2]]);
    expect(filterSourceNodes(nodes, '', false)).toEqual(nodes);
  });
});
