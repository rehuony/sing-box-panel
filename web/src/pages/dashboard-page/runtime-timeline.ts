import type {
  RuntimeHistoryPage,
  RuntimeTransition,
  RuntimeTransitionState,
} from '@/api/api-client';

export interface TimelineSegment {
  id: string;
  end: number;
  start: number;
  reason: string;
  state: RuntimeTransitionState;
}

interface PreparedTransition {
  occurredAt: number;
  effectiveAt: number;
  transition: RuntimeTransition;
}

function prepareTransitions(transitions: Array<RuntimeTransition | undefined>): PreparedTransition[] {
  return transitions
    .flatMap((transition) => {
      if (transition === undefined) return [];
      const occurredAt = Date.parse(transition.occurred_at);
      if (!Number.isFinite(occurredAt)) return [];
      let effectiveAt = occurredAt;
      if (transition.state === 'unknown' && transition.uncertain_since !== undefined) {
        const uncertainSince = Date.parse(transition.uncertain_since);
        if (Number.isFinite(uncertainSince) && uncertainSince <= occurredAt) {
          effectiveAt = uncertainSince;
        }
      }
      return [{ effectiveAt, occurredAt, transition }];
    })
    .sort((left, right) => {
      if (left.occurredAt !== right.occurredAt) return left.occurredAt - right.occurredAt;
      if (left.transition.id === right.transition.id) return 0;
      return left.transition.id < right.transition.id ? -1 : 1;
    });
}

function uncertaintyAwareBoundaries(prepared: PreparedTransition[]): PreparedTransition[] {
  const boundaries: PreparedTransition[] = [];
  for (const item of prepared) {
    if (item.transition.state === 'unknown') {
      while (boundaries.length > 0 && boundaries[boundaries.length - 1].effectiveAt >= item.effectiveAt) {
        boundaries.pop();
      }
    }
    boundaries.push(item);
  }
  return boundaries;
}

function segmentFor(
  current: RuntimeTransition | undefined,
  start: number,
  end: number,
): TimelineSegment {
  return {
    id: `${current?.id ?? 'unknown'}-${start}`,
    end,
    start,
    reason: current?.reason ?? 'history_unavailable',
    state: current?.state ?? 'unknown',
  };
}

export function buildRuntimeTimeline(
  history: RuntimeHistoryPage | null,
  from: number,
  to: number,
): TimelineSegment[] {
  if (history === null || to <= from) return [];

  const truncated = history.next !== undefined;
  const loaded = prepareTransitions(history.items);
  const prepared = truncated
    ? loaded
    : prepareTransitions([history.preceding, ...history.items]);
  const boundaries = uncertaintyAwareBoundaries(prepared);
  const segments: TimelineSegment[] = [];
  let cursor = from;
  let current: RuntimeTransition | undefined;
  let boundaryIndex = 0;

  if (truncated) {
    const oldestLoaded = loaded[0];
    if (oldestLoaded === undefined) return [segmentFor(undefined, from, to)];
    const derivationStart = Math.min(to, Math.max(from, oldestLoaded.occurredAt));
    if (cursor < derivationStart) {
      segments.push(segmentFor(undefined, cursor, derivationStart));
      cursor = derivationStart;
    }
  }

  while (boundaryIndex < boundaries.length && boundaries[boundaryIndex].effectiveAt <= cursor) {
    current = boundaries[boundaryIndex].transition;
    boundaryIndex += 1;
  }

  for (; boundaryIndex < boundaries.length; boundaryIndex += 1) {
    const boundary = boundaries[boundaryIndex];
    if (boundary.effectiveAt >= to) break;
    if (boundary.effectiveAt > cursor) {
      segments.push(segmentFor(current, cursor, boundary.effectiveAt));
    }
    current = boundary.transition;
    cursor = boundary.effectiveAt;
  }

  if (cursor < to) segments.push(segmentFor(current, cursor, to));
  return segments.filter((segment) => segment.end > segment.start);
}
