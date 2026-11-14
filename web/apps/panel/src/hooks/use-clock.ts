import { create } from 'zustand';
import { useEffect } from 'react';

/** How often durations such as uptimes and last-seen times count on. */
const TICK_MS = 30_000;

const useClockStore = create<{ now: number }>()(() => ({ now: Date.now() }));
let subscribers = 0;
let timer: ReturnType<typeof setInterval> | undefined;

/**
 * The time durations count to, shared by every reader and moving on every
 * 30 seconds, so many readers redraw together and only when it moves.
 */
export function useClock(): number {
  useEffect(() => {
    subscribers += 1;
    if (subscribers === 1) {
      useClockStore.setState({ now: Date.now() });
      timer = setInterval(() => useClockStore.setState({ now: Date.now() }), TICK_MS);
    }
    return () => {
      subscribers -= 1;
      if (subscribers === 0) clearInterval(timer);
    };
  }, []);
  return useClockStore(state => state.now);
}
