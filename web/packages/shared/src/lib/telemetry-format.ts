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
  if (value === null || !Number.isFinite(value) || value < 0) return EM_DASH;
  return `${formatBytes(value, locale)}${perSecond}`;
}

interface DurationLabels {
  day: string;
  hour: string;
  minute: string;
  second: string;
}

/** A span of time in its two largest units, such as "3d 4h"; negative spans count as none. */
export function formatDuration(milliseconds: number, labels: DurationLabels): string {
  const seconds = Number.isFinite(milliseconds) && milliseconds > 0 ? Math.floor(milliseconds / 1_000) : 0;
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  if (days > 0) return `${days}${labels.day} ${hours}${labels.hour}`;
  if (hours > 0) return `${hours}${labels.hour} ${minutes}${labels.minute}`;
  if (minutes > 0) return `${minutes}${labels.minute}`;
  return `${seconds}${labels.second}`;
}

export function formatUptime(
  startedAt: string | undefined,
  now: number,
  labels: DurationLabels,
): string {
  if (startedAt === undefined) return `0${labels.second}`;
  return formatDuration(now - new Date(startedAt).getTime(), labels);
}

/** The shares where a meter's scale reaches warning and danger. */
export interface MeterLimits {
  danger: number;
  warning: number;
}

/** Where a load turns warning and danger. */
export const SHARE_LIMITS: MeterLimits = { danger: 95, warning: 80 };

/** The tone a share takes as it nears full: warning from 80%, danger from 95%. */
export function shareTone(percent: number | null): 'danger' | 'warning' | undefined {
  if (percent === null) return undefined;
  return percent >= SHARE_LIMITS.danger ? 'danger' : percent >= SHARE_LIMITS.warning ? 'warning' : undefined;
}
