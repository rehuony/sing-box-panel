import type { TFunction } from 'i18next';

/** A log entry's or a runtime transition's translated event name, else its own message or code. */
export function logTitle(item: { code: string; message?: string }, t: TFunction): string {
  return t(`productLogs.events.${item.code}`, { defaultValue: item.message || item.code });
}

/** Whether a log time falls in the year of now. */
export function inCurrentYear(value: string, now = new Date()): boolean {
  return new Date(value).getFullYear() === now.getFullYear();
}

/**
 * A log line's time in a table: month and day with the time, the year only
 * when it is not the current one, and the seconds unless left out.
 */
export function formatLogTimeCompact(
  value: string,
  language?: string,
  { seconds = true, now = new Date() } = {},
): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(language, {
    ...(inCurrentYear(value, now) ? {} : { year: 'numeric' } as const),
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
    ...(seconds ? { second: '2-digit' } as const : {}),
  }).format(date);
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
