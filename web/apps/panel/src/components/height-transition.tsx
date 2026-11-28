import type { ReactNode } from 'react';

import { useLayoutEffect, useRef } from 'react';

/** A motion token in milliseconds, such as `--duration-medium`, or the fallback where it cannot be read. */
function durationToken(name: string, fallback: number): number {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const seconds = /^([\d.]+)s$/.exec(value);
  const milliseconds = /^([\d.]+)ms$/.exec(value);
  if (seconds) return Number(seconds[1]) * 1000;
  if (milliseconds) return Number(milliseconds[1]);
  return fallback;
}

/**
 * Eases its height when what it holds grows or shrinks, as when a dialog's
 * step changes or a row joins it, on the movement curve in the medium
 * duration; reduced motion takes the new height at once. It clips only while
 * it moves, so focus marks around fields show at rest.
 */
export function HeightTransition({ children, className }: { children: ReactNode; className?: string }) {
  const outerRef = useRef<HTMLDivElement>(null);
  const innerRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const outer = outerRef.current;
    const inner = innerRef.current;
    if (!outer || !inner) return;
    let last = inner.offsetHeight;
    let running: Animation | null = null;
    // Resize callbacks run before paint, so the new height never shows before the animation starts from the old.
    const observer = new ResizeObserver(() => {
      const next = inner.offsetHeight;
      const from = running ? outer.getBoundingClientRect().height : last;
      last = next;
      if (from === next || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
      running?.cancel();
      outer.style.overflow = 'hidden';
      const easing = getComputedStyle(document.documentElement).getPropertyValue('--ease-out').trim() || 'ease-out';
      running = outer.animate([{ height: `${from}px` }, { height: `${next}px` }], { duration: durationToken('--duration-medium', 280), easing });
      running.onfinish = () => {
        running = null;
        outer.style.overflow = '';
      };
    });
    observer.observe(inner);
    return () => {
      observer.disconnect();
      running?.cancel();
    };
  }, []);
  return (
    <div ref={outerRef}>
      <div className={className} ref={innerRef}>{children}</div>
    </div>
  );
}
