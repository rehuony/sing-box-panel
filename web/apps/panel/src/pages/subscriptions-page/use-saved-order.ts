import type { QueryKey } from '@tanstack/react-query';

import { useMemo, useState } from 'react';
import { arrayMove } from '@dnd-kit/sortable';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import { ApiRequestError } from '@/api/api-client';
import { toastFailure } from '@/components/error-notice';

/** The items in the order of the IDs, then any item the IDs miss. */
function withOrder<T extends { id: string }>(items: readonly T[], ids: readonly string[]): T[] {
  const byID = new Map(items.map(item => [item.id, item]));
  const placed = new Set(ids);
  return [...ids.flatMap(id => byID.get(id) ?? []), ...items.filter(item => !placed.has(item.id))];
}

/**
 * A list the server keeps in an order its user sets by dragging. A drop shows
 * the new order at once and saves it whole; drops save one after another, so
 * the last one wins, and a list read meanwhile gives way to it. An order the
 * server finds stale says so and the list is read again; any other failure
 * says why, and the list is read again too.
 */
export function useSavedOrder<T extends { id: string }>({ failed, items, queryKey, save, stale }: {
  items: readonly T[] | undefined;
  /** The list's query, which a saved order updates and a refused one reads again. */
  queryKey: QueryKey;
  save: (ids: string[]) => Promise<void>;
  /** What a stale order says. */
  stale: string;
  /** What another failure says, before why. */
  failed: string;
}) {
  const cache = useQueryClient();
  const [order, setOrder] = useState<readonly string[] | null>(null);
  const saveOrder = useMutation({
    scope: { id: `${JSON.stringify(queryKey)}-order` },
    mutationFn: (ids: readonly string[]) => save([...ids]),
    onSuccess: async (_saved, ids) => {
      await cache.cancelQueries({ queryKey, exact: true });
      cache.setQueryData<T[]>(queryKey, current => current && withOrder(current, ids));
    },
    onError: (error) => {
      if (error instanceof ApiRequestError && error.status === 409) toast.add({ title: stale, type: 'warning' });
      else toastFailure(failed, error);
      void cache.invalidateQueries({ queryKey, exact: true });
    },
    onSettled: (_saved, _error, ids) => setOrder(current => current === ids ? null : current),
  });
  const ordered = useMemo(() => (items && order ? withOrder(items, order) : items ?? []), [items, order]);

  /** Moves an item to another's place and saves the order. */
  function move(active: string, over: string) {
    const ids = ordered.map(item => item.id);
    const from = ids.indexOf(active);
    const to = ids.indexOf(over);
    if (from < 0 || to < 0 || from === to) return;
    const next = arrayMove(ids, from, to);
    setOrder(next);
    saveOrder.mutate(next);
  }

  return { move, ordered };
}
