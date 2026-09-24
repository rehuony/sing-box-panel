import type { TFunction } from 'i18next';

import type { PanelLog } from '@/api/api-client';

import { productLogs as english } from '@/i18n/locales/en/product-logs';
import { productLogs as chinese } from '@/i18n/locales/zh-CN/product-logs';

const eventCodes = Object.keys(english.events) as (keyof typeof english.events)[];

export function matchingEventCodes(search: string): string[] | undefined {
  const query = search.trim().toLowerCase();
  if (!query) return undefined;
  const codes = eventCodes.filter(code =>
    [english.events[code], chinese.events[code]].some(title => title.toLowerCase().includes(query)),
  );
  return codes.length ? codes : undefined;
}

export function logTitle(item: PanelLog, t: TFunction): string {
  return t(`productLogs.events.${item.code}`, { defaultValue: item.message || item.code });
}

export function formatLogTime(value: string, language?: string, detailed = false): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(language, {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
    ...(detailed ? { fractionalSecondDigits: 3, timeZoneName: 'shortOffset' } as const : {}),
  }).format(date);
}

export function metadataLabel(key: string, t: TFunction): string {
  return t(`productLogs.metadata.${key}`, { defaultValue: key });
}

export function metadataValue(key: string, value: unknown, item: PanelLog, t: TFunction, language?: string): string {
  if (key === 'error' && typeof value === 'string' && typeof item.metadata.error_code === 'string') {
    return t(`productLogs.errors.${item.metadata.error_code}`, { defaultValue: String(value) });
  }
  if (['process_started_at', 'started_at', 'uncertain_since'].includes(key) && typeof value === 'string') {
    return formatLogTime(value, language, true);
  }
  return typeof value === 'string' ? value : JSON.stringify(value, null, 2) ?? '—';
}
