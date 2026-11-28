import { cn } from '@shared/lib/utils';
import { describe, expect, it } from 'vitest';

describe('cn', () => {
  it('keeps the type scale steps of its own beside a text colour, and lets a later size replace them', () => {
    expect(cn('text-compact text-[var(--color-text-secondary)]')).toBe('text-compact text-[var(--color-text-secondary)]');
    expect(cn('text-2xs text-muted-foreground')).toBe('text-2xs text-muted-foreground');
    expect(cn('text-compact', 'text-xs')).toBe('text-xs');
  });
});
