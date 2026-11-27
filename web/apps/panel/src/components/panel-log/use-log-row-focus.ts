import type { KeyboardEvent } from 'react';

import { useState } from 'react';

const moves: Record<string, (index: number, last: number) => number> = {
  ArrowDown: (index, last) => Math.min(last, index + 1),
  ArrowUp: index => Math.max(0, index - 1),
  End: (_index, last) => last,
  Home: () => 0,
};

/**
 * Keeps a log table one Tab stop: the line that last held focus, or the
 * first, takes it, and the arrow, Home and End keys move between the lines'
 * titles, as the rule lines do.
 */
export function useLogRowFocus(ids: readonly string[]) {
  const [current, setCurrent] = useState<string | null>(null);
  const active = current !== null && ids.includes(current) ? current : ids[0];
  function onKeyDown(event: KeyboardEvent<HTMLElement>) {
    const move = moves[event.key];
    if (!move) return;
    const titles = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button[data-row-title]')];
    const index = titles.indexOf(document.activeElement as HTMLButtonElement);
    if (index < 0) return;
    event.preventDefault();
    titles[move(index, titles.length - 1)]?.focus();
  }
  return {
    onFocus: setCurrent,
    onKeyDown,
    tabIndex: (id: string) => (id === active ? 0 : -1),
  };
}
