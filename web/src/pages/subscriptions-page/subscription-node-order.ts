import { arrayMove } from '@dnd-kit/sortable';

import type { SubscriptionNodeSummary } from '@/api/api-client';

export function orderSourceNodes(nodes: SubscriptionNodeSummary[], savedOrder: string[]): SubscriptionNodeSummary[] {
  const byID = new Map(nodes.map(node => [node.id, node]));
  const order = [...new Set([...savedOrder.filter(id => byID.has(id)), ...byID.keys()])];
  return order.map(id => byID.get(id)!);
}

export function filterSourceNodes(nodes: SubscriptionNodeSummary[], search: string, selecting: boolean) {
  return nodes.filter(node =>
    (!selecting || (!node.hidden && node.available))
    && `${node.name} ${node.source_name} ${node.type}`.toLowerCase().includes(search.toLowerCase()));
}

// Searching only reorders visible positions; hidden cards keep their slots.
export function reorderVisibleNodes(order: string[], visible: string[], active: string, over: string): string[] {
  const from = visible.indexOf(active);
  const to = visible.indexOf(over);
  if (from < 0 || to < 0 || from === to) return order;
  const moved = arrayMove(visible, from, to);
  const shown = new Set(visible);
  let index = 0;
  return order.map((id) => shown.has(id) ? moved[index++] : id);
}

const storagePrefix = 'sing-box-panel.source-node-order.';

export function readSourceNodeOrder(sourceID?: string): string[] {
  if (!sourceID) return [];
  try {
    const value: unknown = JSON.parse(window.localStorage.getItem(storagePrefix + sourceID) ?? '[]');
    return Array.isArray(value) && value.length <= 10_000 && value.every((id) => typeof id === 'string')
      ? [...new Set(value as string[])]
      : [];
  } catch {
    return [];
  }
}

export function saveSourceNodeOrder(sourceID: string, order: string[]): void {
  window.localStorage.setItem(storagePrefix + sourceID, JSON.stringify(order));
}
