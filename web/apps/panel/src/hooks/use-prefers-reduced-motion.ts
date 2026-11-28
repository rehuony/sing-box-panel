import { useSyncExternalStore } from 'react';

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)';

/** The one query every caller reads, made on first use; null where the browser has none. */
let reducedMotionQuery: MediaQueryList | null | undefined;
const subscribers = new Set<() => void>();

function mediaQuery(): MediaQueryList | null {
  if (reducedMotionQuery === undefined) {
    reducedMotionQuery = typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia(REDUCED_MOTION_QUERY)
      : null;
  }
  return reducedMotionQuery;
}

function notify() {
  for (const subscriber of subscribers) subscriber();
}

/** Subscribes to the query through one listener, however many components read it. */
function subscribeToReducedMotion(onStoreChange: () => void): () => void {
  const query = mediaQuery();
  if (!query) return () => undefined;
  if (subscribers.size === 0) query.addEventListener('change', notify);
  subscribers.add(onStoreChange);
  return () => {
    subscribers.delete(onStoreChange);
    if (subscribers.size === 0) query.removeEventListener('change', notify);
  };
}

export function getReducedMotionSnapshot(): boolean {
  return mediaQuery()?.matches ?? true;
}

export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(subscribeToReducedMotion, getReducedMotionSnapshot, () => true);
}
