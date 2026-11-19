import { describe, expect, it } from 'vitest';

import { reorderVisibleNodes } from '@/pages/subscriptions-page/subscription-node-order';

describe('visible reordering', () => {
  it('keeps filtered-out slots when moving visible cards', () => {
    const order = ['node:a', 'builtin:direct', 'node:b', 'builtin:reject'];
    expect(reorderVisibleNodes(order, ['node:a', 'node:b'], 'node:b', 'node:a'))
      .toEqual(['node:b', 'builtin:direct', 'node:a', 'builtin:reject']);
    expect(reorderVisibleNodes(order, ['node:a'], 'unknown', 'node:a')).toBe(order);
  });
});
