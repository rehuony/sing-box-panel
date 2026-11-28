const dateFormats = new Map<string, Intl.DateTimeFormat>();
const relativeFormats = new Map<string, Intl.RelativeTimeFormat>();

/**
 * The date and time format for a locale and options, built once and kept, as
 * numberFormat keeps number formats: lists and panes format on every render.
 */
export function dateTimeFormat(locale: string, options: Intl.DateTimeFormatOptions = {}): Intl.DateTimeFormat {
  const key = `${locale} ${JSON.stringify(options)}`;
  let format = dateFormats.get(key);
  if (format === undefined) {
    format = new Intl.DateTimeFormat(locale, options);
    dateFormats.set(key, format);
  }
  return format;
}

function relativeTimeFormat(locale: string): Intl.RelativeTimeFormat {
  let format = relativeFormats.get(locale);
  if (format === undefined) {
    format = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
    relativeFormats.set(locale, format);
  }
  return format;
}

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
  const format = relativeTimeFormat(locale);
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size) return format.format(Math.trunc(seconds / size), unit);
  }
  return format.format(seconds, 'second');
}

/** A full date and time for a hover title; empty when the time is unknown. */
export function formatDateTime(value: string | null | undefined, locale: string): string {
  const time = value ? Date.parse(value) : Number.NaN;
  if (!Number.isFinite(time)) return '';
  return dateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(time);
}

/**
 * A compact date and time, month and day with hours and minutes, adding the
 * year only when it is not the current one; empty when the time is unknown.
 */
export function formatShortDateTime(value: string | null | undefined, locale: string, now = Date.now()): string {
  const time = value ? Date.parse(value) : Number.NaN;
  if (!Number.isFinite(time)) return '';
  const year = new Date(time).getFullYear() === new Date(now).getFullYear() ? undefined : 'numeric';
  return dateTimeFormat(locale, { day: 'numeric', hour: '2-digit', minute: '2-digit', month: 'short', year }).format(time);
}
