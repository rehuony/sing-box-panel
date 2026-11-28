import type { ClassValue } from 'clsx';

import { clsx } from 'clsx';
import { extendTailwindMerge } from 'tailwind-merge';

/**
 * Merges classes knowing the type scale's own steps, so `text-compact` and
 * `text-2xs` read as sizes and survive a text colour beside them.
 */
const twMerge = extendTailwindMerge({ extend: { theme: { text: ['2xs', 'compact'] } } });

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
