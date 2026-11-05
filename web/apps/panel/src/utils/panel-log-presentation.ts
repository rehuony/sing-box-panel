import type { TFunction } from 'i18next';

import type { PanelLog } from '@/api/api-client';

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
