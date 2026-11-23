const units = [
  ['year', 31_536_000],
  ['month', 2_592_000],
  ['week', 604_800],
  ['day', 86_400],
  ['hour', 3_600],
  ['minute', 60],
] as const;

/** A past or future time relative to now, such as "3 hours ago"; "—" when the time is unknown. */
export function formatRelativeTime(value: string | null | undefined, locale: string, now = Date.now()): string {
  const time = value ? Date.parse(value) : Number.NaN;
  if (!Number.isFinite(time)) return '—';
  const seconds = Math.round((time - now) / 1_000);
  const format = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size) return format.format(Math.trunc(seconds / size), unit);
  }
  return format.format(seconds, 'second');
}

/** A full date and time for a hover title; empty when the time is unknown. */
export function formatDateTime(value: string | null | undefined, locale: string): string {
  const time = value ? Date.parse(value) : Number.NaN;
  if (!Number.isFinite(time)) return '';
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(time);
}

/**
 * A compact date and time, month and day with hours and minutes, adding the
 * year only when it is not the current one; empty when the time is unknown.
 */
export function formatShortDateTime(value: string | null | undefined, locale: string, now = Date.now()): string {
  const time = value ? Date.parse(value) : Number.NaN;
  if (!Number.isFinite(time)) return '';
  const year = new Date(time).getFullYear() === new Date(now).getFullYear() ? undefined : 'numeric';
  return new Intl.DateTimeFormat(locale, { day: 'numeric', hour: '2-digit', minute: '2-digit', month: 'short', year }).format(time);
}
