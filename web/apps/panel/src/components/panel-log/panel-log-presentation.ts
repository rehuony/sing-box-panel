import type { TFunction } from 'i18next';

import { formatBytes } from '@shared/lib/telemetry-format';

import { productLogs as english } from '@/i18n/locales/en/product-logs';
import { productLogs as chinese } from '@/i18n/locales/zh-CN/product-logs';
import { formatLogTime, inCurrentYear } from '@/utils/panel-log-presentation';

const eventCodes = Object.keys(english.events) as (keyof typeof english.events)[];

/** The time column's width: room for the year when a listed line is from another one. */
export function logTimeColumnClass(times: readonly string[]): string {
  return times.every(time => inCurrentYear(time)) ? 'w-36 max-sm:w-28' : 'w-44 max-sm:w-38';
}

export function matchingEventCodes(search: string): string[] | undefined {
  const query = search.trim().toLowerCase();
  if (!query) return undefined;
  const codes = eventCodes.filter(code =>
    [english.events[code], chinese.events[code]].some(title => title.toLowerCase().includes(query)),
  );
  return codes.length ? codes : undefined;
}

export function metadataLabel(key: string, t: TFunction): string {
  return t(`productLogs.metadata.${key}`, { defaultValue: key });
}

export function metadataValue(
  key: string,
  value: unknown,
  metadata: Record<string, unknown>,
  t: TFunction,
  language?: string,
): string {
  if (key === 'error' && typeof value === 'string' && typeof metadata.error_code === 'string') {
    return t(`productLogs.errors.${metadata.error_code}`, { defaultValue: String(value) });
  }
  if (['process_started_at', 'started_at', 'uncertain_since'].includes(key) && typeof value === 'string') {
    return formatLogTime(value, language, true);
  }
  if (key === 'freed_bytes' && typeof value === 'number') return formatBytes(value, language);
  return typeof value === 'string' ? value : JSON.stringify(value, null, 2) ?? '—';
}
