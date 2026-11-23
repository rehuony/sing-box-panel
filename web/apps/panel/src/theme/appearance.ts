import { isThemePreference } from '@shared/theme/theme';

import type { AppearanceSettings } from '@/api/api-client';

export const DEFAULT_APPEARANCE: AppearanceSettings = { theme: 'system', color: '#6D4ED1', radius: 12 };
export const THEME_PRESETS = ['#6D4ED1', '#2563EB', '#0891B2', '#15803D', '#C65B13', '#BE185D'] as const;

export function readInitialAppearance(): AppearanceSettings | null {
  const content = document.querySelector<HTMLMetaElement>('meta[name="sing-box-panel-appearance"]')?.content;
  if (!content || content === '__SBP_APPEARANCE__') return null;
  try {
    const value: unknown = JSON.parse(content);
    if (typeof value !== 'object' || value === null) return null;
    const { theme, color, radius } = value as Partial<AppearanceSettings>;
    if (!isThemePreference(theme) || typeof color !== 'string' || !/^#[\dA-F]{6}$/i.test(color)
      || typeof radius !== 'number' || !Number.isInteger(radius) || radius < 0 || radius > 32) {
      return null;
    }
    return { theme, color, radius };
  } catch {
    return null;
  }
}

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
  // The card surface, the same under every accent: #25262f and #f8f8fa.
  const surface: RGB = dark ? [37, 38, 47] : [248, 248, 250];
  const soft = hex(mix(base, surface, dark ? 0.84 : 0.91));
  let foreground = base;
  const target: RGB = dark ? [255, 255, 255] : [0, 0, 0];
  // Preserve hue while ensuring accent text is readable even for white/neon choices.
  for (let step = 0; step <= 100; step += 1) {
    foreground = mix(base, target, step / 100);
    if (contrastRatio(hex(foreground), soft) >= 4.6 && contrastRatio(hex(foreground), hex(surface)) >= 4.6) break;
  }
  const accent = hex(foreground);
  const radius = Number.isFinite(appearance.radius)
    ? Math.max(0, Math.min(32, Math.round(appearance.radius)))
    : DEFAULT_APPEARANCE.radius;
  const onAccent = contrastRatio(accent, '#ffffff') >= 4.5 ? '#ffffff' : '#17151e';
  return {
    '--appearance-color': color,
    '--primary': accent,
    '--primary-foreground': onAccent,
    '--accent': soft,
    '--radius-card': `${radius}px`,
    '--radius-control': `${radius / 2}px`,
    '--radius-window': `${Math.min(32, radius * 7 / 6)}px`,
  };
}

export function applyAppearance(appearance: AppearanceSettings, dark: boolean): void {
  for (const [property, value] of Object.entries(appearanceTokens(appearance, dark))) {
    document.documentElement.style.setProperty(property, value);
  }
}
