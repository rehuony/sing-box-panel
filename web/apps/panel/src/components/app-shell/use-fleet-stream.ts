import { create } from 'zustand';
import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import type { FleetStatus, FleetStreamFrame } from '@/api/api-client';

import { queries } from '@/api/queries';
import { followStream } from '@/api/stream-connection';
import { useApiClient } from '@/api/api-client-context';
import { usePageVisible } from '@/hooks/use-page-visible';

type NodeStatus = FleetStatus['items'][number];

function foldASCII(value: string): string {
  return value.replace(/[A-Z]/g, letter => letter.toLowerCase());
}

/** Compares by code point, as the panel compares UTF-8 bytes. */
function compareCodePoints(left: string, right: string): number {
  const leftPoints = [...left];
  const rightPoints = [...right];
  for (let index = 0; index < Math.min(leftPoints.length, rightPoints.length); index++) {
    const difference = leftPoints[index]!.codePointAt(0)! - rightPoints[index]!.codePointAt(0)!;
    if (difference !== 0) return difference;
  }
  return leftPoints.length - rightPoints.length;
}

/** The panel's fleet order: names ignoring ASCII case, then IDs. */
function compareNodes(left: NodeStatus, right: NodeStatus): number {
  return compareCodePoints(foldASCII(left.node.name), foldASCII(right.node.name))
    || compareCodePoints(left.node.id, right.node.id);
}

/**
 * Applies one fleet stream frame to the fleet a client holds: a reset frame
 * replaces it; any other frame replaces the nodes it carries and drops the
 * ones it names as removed, keeping the panel's order.
 */
export function applyFleetFrame(current: FleetStatus | undefined, frame: FleetStreamFrame): FleetStatus {
  if (frame.reset || !current) return { items: frame.items };
  const replaced = new Set([...frame.removed, ...frame.items.map(item => item.node.id)]);
  const items = current.items.filter(item => !replaced.has(item.node.id)).concat(frame.items);
  return { items: items.sort(compareNodes) };
}

/** Whether the fleet stream stayed down, so the fleet's live values no longer update. */
export const useFleetStreamState = create<{ down: boolean }>()(() => ({ down: false }));

/**
 * Feeds the fleet status query from the panel's shared fleet stream while
 * the page is visible, so every fleet view reads one live fleet. A stream
 * that stays down hands over to a read of the status, whose failure marks
 * the views' last reading as no longer updating.
 */
export function useFleetStream() {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const visible = usePageVisible();
  useEffect(() => {
    if (!visible) return;
    const { queryKey } = queries.fleetStatus(client);
    return followStream(
      signal => client.streamFleetStatus(signal),
      (frame) => {
        useFleetStreamState.setState({ down: false });
        queryClient.setQueryData<FleetStatus>(queryKey, current => applyFleetFrame(current, frame));
      },
      () => {
        useFleetStreamState.setState({ down: true });
        void queryClient.refetchQueries({ queryKey, exact: true });
      },
      () => {},
    );
  }, [client, queryClient, visible]);
}
