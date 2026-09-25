export const EM_DASH = '—';

export function formatBytes(value: number | null | undefined, locale = 'en'): string {
  if (value === null || value === undefined || !Number.isFinite(value) || value < 0) return EM_DASH;
  if (value < 1_024) return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(value)} B`;
  const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let amount = value / 1_024;
  let unit = units[0];
  for (let index = 1; index < units.length && amount >= 1_024; index += 1) {
    amount /= 1_024;
    unit = units[index];
  }
  return `${new Intl.NumberFormat(locale, {
    maximumFractionDigits: amount >= 10 ? 0 : 1,
  }).format(amount)} ${unit}`;
}

export function formatRate(value: number | null, locale = 'en', perSecond = '/s'): string {
  const rate = value !== null && Number.isFinite(value) && value >= 0 ? value : 0;
  return `${formatBytes(rate, locale)}${perSecond}`;
}

interface DurationLabels {
  day: string;
  hour: string;
  minute: string;
  second: string;
}

export function formatUptime(
  startedAt: string | undefined,
  now: number,
  labels: DurationLabels,
): string {
  if (startedAt === undefined) return `0${labels.second}`;
  const started = new Date(startedAt).getTime();
  if (!Number.isFinite(started) || started > now) return `0${labels.second}`;
  const seconds = Math.floor((now - started) / 1_000);
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  if (days > 0) return `${days}${labels.day} ${hours}${labels.hour}`;
  if (hours > 0) return `${hours}${labels.hour} ${minutes}${labels.minute}`;
  if (minutes > 0) return `${minutes}${labels.minute}`;
  return `${seconds}${labels.second}`;
}
