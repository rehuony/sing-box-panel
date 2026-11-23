import { arrayMove } from '@dnd-kit/sortable';

/**
 * Moves one visible item onto another's position. Items a filter hides keep
 * their slots, so only the visible ones change places.
 */
export function reorderVisibleNodes(order: string[], visible: string[], active: string, over: string): string[] {
  const from = visible.indexOf(active);
  const to = visible.indexOf(over);
  if (from < 0 || to < 0 || from === to) return order;
  const moved = arrayMove(visible, from, to);
  const shown = new Set(visible);
  let index = 0;
  return order.map((id) => shown.has(id) ? moved[index++] : id);
}
