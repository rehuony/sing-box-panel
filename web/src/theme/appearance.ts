import type { AppearanceSettings } from '@/api/api-client';

export const DEFAULT_APPEARANCE: AppearanceSettings = { theme: 'light', color: '#6D4ED1', radius: 24 };
export const THEME_PRESETS = ['#6D4ED1', '#2563EB', '#0891B2', '#15803D', '#C65B13', '#BE185D'] as const;

type RGB = [number, number, number];
const rgb = (hex: string): RGB => [1, 3, 5].map(start => Number.parseInt(hex.slice(start, start + 2), 16)) as RGB;
const hex = (value: RGB) => `#${value.map(channel => Math.round(channel).toString(16).padStart(2, '0')).join('')}`;
function mix(value: RGB, target: RGB, weight: number): RGB {
  return value.map((channel, i) => channel * (1 - weight) + target[i]! * weight) as RGB;
}

function luminance(value: RGB): number {
  const linear = value.map(channel => {
    const n = channel / 255;
    return n <= 0.04045 ? n / 12.92 : ((n + 0.055) / 1.055) ** 2.4;
  });
  return linear[0]! * 0.2126 + linear[1]! * 0.7152 + linear[2]! * 0.0722;
}

export function contrastRatio(first: string, second: string): number {
  const a = luminance(rgb(first));
  const b = luminance(rgb(second));
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

export function appearanceTokens(appearance: AppearanceSettings, dark = false): Record<string, string> {
  const color = /^#[\dA-F]{6}$/i.test(appearance.color) ? appearance.color : DEFAULT_APPEARANCE.color;
  const base = rgb(color);
  const surface: RGB = dark ? [24, 23, 30] : [255, 255, 255];
  const soft = hex(mix(base, surface, dark ? 0.84 : 0.91));
  let foreground = base;
  const target: RGB = dark ? [255, 255, 255] : [0, 0, 0];
  // Preserve hue while ensuring accent text is readable even for white/neon choices.
  for (let step = 0; step <= 100; step += 1) {
    foreground = mix(base, target, step / 100);
    if (contrastRatio(hex(foreground), soft) >= 4.6 && contrastRatio(hex(foreground), hex(surface)) >= 4.6) break;
  }
  const accent = hex(foreground);
  const radius = Number.isFinite(appearance.radius) ? Math.max(0, Math.min(32, Math.round(appearance.radius))) : 24;
  return {
    '--appearance-color': color,
    '--color-accent': accent,
    '--primary': accent,
    '--primary-foreground': contrastRatio(accent, '#ffffff') >= 4.5 ? '#ffffff' : '#17151e',
    '--ring': accent,
    '--sidebar-primary': accent,
    '--sidebar-primary-foreground': contrastRatio(accent, '#ffffff') >= 4.5 ? '#ffffff' : '#17151e',
    '--sidebar-accent': soft,
    '--sidebar-accent-foreground': accent,
    '--sidebar-ring': accent,

    '--color-accent-hover': hex(mix(foreground, target, 0.12)),
    '--color-accent-soft': soft,
    '--color-accent-ink': contrastRatio(accent, '#ffffff') >= 4.5 ? '#ffffff' : '#17151e',
    '--color-focus': accent,
    '--color-paper': hex(mix(base, surface, 0.98)),
    '--color-paper-2': hex(mix(base, surface, 0.96)),
    '--color-paper-3': hex(mix(base, surface, 0.92)),
    '--color-rule': hex(mix(base, surface, dark ? 0.58 : 0.74)),
    '--card': hex(mix(base, surface, 0.995)),
    '--popover': hex(mix(base, surface, 0.995)),
    '--muted': hex(mix(base, surface, 0.96)),
    '--color-rule-2': hex(mix(base, surface, dark ? 0.68 : 0.84)),
    '--border': hex(mix(base, surface, dark ? 0.68 : 0.84)),
    '--input': hex(mix(base, surface, dark ? 0.68 : 0.84)),
    '--radius-card': `${radius}px`,
    '--radius-panel': `${radius}px`,
    '--radius-control': `${radius / 2}px`,
    '--radius-sm': `${radius / 2}px`,
    '--radius-window': `${Math.min(32, radius * 7 / 6)}px`,
  };
}

export function applyAppearance(appearance: AppearanceSettings, dark: boolean): void {
  for (const [property, value] of Object.entries(appearanceTokens(appearance, dark))) {
    document.documentElement.style.setProperty(property, value);
  }
}
