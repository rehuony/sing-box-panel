import { describe, expect, it } from 'vitest';

import { appearanceTokens, contrastRatio, DEFAULT_APPEARANCE, readInitialAppearance, THEME_PRESETS } from '@/theme/appearance';

describe('initial appearance', () => {
  it('ignores missing, development, and malformed metadata', () => {
    expect(readInitialAppearance()).toBeNull();
    const meta = document.createElement('meta');
    meta.name = 'sing-box-panel-appearance';
    document.head.append(meta);
    try {
      for (const content of ['__SBP_APPEARANCE__', 'null', 'invalid', '{}',
        ...[{ theme: 'sepia' }, { color: 'red' }, { radius: -1 }, { radius: 33 }, { radius: 1.5 }]
          .map(value => JSON.stringify({ ...DEFAULT_APPEARANCE, ...value }))]) {
        meta.content = content;
        expect(readInitialAppearance()).toBeNull();
      }
      meta.content = JSON.stringify({ theme: 'dark', color: '#C65B13', radius: 0, ignored: true });
      expect(readInitialAppearance()).toEqual({ theme: 'dark', color: '#C65B13', radius: 0 });
    } finally {
      meta.remove();
    }
  });
});

describe('appearance tokens', () => {
  it('defaults to system and derives only the accent roles the stylesheet defaults to', () => {
    expect(DEFAULT_APPEARANCE.theme).toBe('system');
    const names = ['--primary', '--accent', '--color-accent-hover'];
    for (const [dark, expected] of [
      [false, ['#6d4ed1', '#ebe9f6', '#6045b8']],
      [true, ['#a08ce1', '#312c49', '#ab9ae5']],
    ] as const) {
      const tokens = appearanceTokens(DEFAULT_APPEARANCE, dark);
      expect(names.map(name => tokens[name])).toEqual(expected);
      expect(tokens).not.toHaveProperty('--color-canvas');
    }
  });
  it('keeps accent text readable for arbitrary colors in both themes', () => {
    for (const color of [...THEME_PRESETS, '#FFFFFF', '#FFFF00', '#00FF00', '#000000', '#AAAAAA']) {
      for (const dark of [false, true]) {
        const tokens = appearanceTokens({ ...DEFAULT_APPEARANCE, color }, dark);
        expect(contrastRatio(tokens['--primary']!, tokens['--accent']!)).toBeGreaterThanOrEqual(4.5);
        expect(contrastRatio(tokens['--primary']!, tokens['--primary-foreground']!)).toBeGreaterThanOrEqual(4.5);
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
