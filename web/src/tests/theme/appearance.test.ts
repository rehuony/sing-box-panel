import { describe, expect, it } from 'vitest';

import { appearanceTokens, contrastRatio, DEFAULT_APPEARANCE, THEME_PRESETS } from '@/theme/appearance';

describe('appearance tokens', () => {
  it('keeps accent text readable for arbitrary colors in both themes', () => {
    for (const color of [...THEME_PRESETS, '#FFFFFF', '#FFFF00', '#00FF00', '#000000', '#AAAAAA']) {
      for (const dark of [false, true]) {
        const tokens = appearanceTokens({ ...DEFAULT_APPEARANCE, color }, dark);
        expect(contrastRatio(tokens['--color-accent']!, tokens['--color-accent-soft']!)).toBeGreaterThanOrEqual(4.5);
        expect(contrastRatio(tokens['--color-accent']!, tokens['--color-accent-ink']!)).toBeGreaterThanOrEqual(4.5);
        expect(tokens['--appearance-color']).toBe(color);
        expect(tokens).not.toHaveProperty('--color-success');
        expect(tokens).not.toHaveProperty('--color-brand-navy');
      }
    }
  });

  it('preserves square controls and scales shell corners without changing pills', () => {
    const square = appearanceTokens({ ...DEFAULT_APPEARANCE, radius: 0 });
    expect(square['--radius-control']).toBe('0px');
    expect(square['--radius-window']).toBe('0px');
    const large = appearanceTokens({ ...DEFAULT_APPEARANCE, radius: 32 });
    expect(large['--radius-control']).toBe('16px');
    expect(large['--radius-window']).toBe('32px');
    expect(large).not.toHaveProperty('--radius-pill');
  });
});
