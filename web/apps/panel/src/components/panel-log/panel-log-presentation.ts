import type { TFunction } from 'i18next';

import { formatBytes } from '@shared/lib/telemetry-format';

import { messageLine } from '@/utils/message-line';
import { describeProblem } from '@/components/error-notice';
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

/** A build's version as a line can hold it: a development build by its release and commit, any other as written. */
function compactVersion(version: string): string {
  const build = /^(v\d+\.\d+\.\d+)-0\.\d{14}-([0-9a-f]{12})(\+dirty)?$/.exec(version);
  return build ? `${build[1]} · ${build[2]!.slice(0, 7)}${build[3] ?? ''}` : version;
}

/** A duration as a log line reads it: milliseconds below a second, seconds to one decimal above. */
function logDuration(milliseconds: number): string {
  return milliseconds < 1000 ? `${Math.round(milliseconds)} ms` : `${(milliseconds / 1000).toFixed(1)} s`;
}

/**
 * What a log line adds after its event, from the context it recorded: why it
 * failed, in the interface's language where the locales know the code, then
 * the node, version, file, address, attempt, counts, size and duration, as
 * far as one line holds them. The details keep every field as recorded.
 */
export function logContext(metadata: Record<string, unknown>, t: TFunction, options: {
  language?: string;
  /** A node's name by its ID; none in a node's own log, which names no node. */
  nodeName?: (id: string) => string | undefined;
}): string {
  const text = (key: string) => typeof metadata[key] === 'string' && metadata[key] !== '' ? metadata[key] : undefined;
  const count = (key: string) => typeof metadata[key] === 'number' && Number.isFinite(metadata[key]) ? metadata[key] : undefined;
  const parts: string[] = [];
  const code = text('error_code');
  if (code) parts.push(messageLine(describeProblem(code, code)));
  const node = text('node_id');
  if (node && options.nodeName) parts.push(options.nodeName(node) ?? node.replace(/^node_/, '').slice(0, 8));
  const version = text('version');
  if (version) parts.push(compactVersion(version));
  for (const key of ['file', 'name', 'address']) {
    const value = text(key);
    if (value) parts.push(value);
  }
  const attempt = count('attempt');
  const maximum = count('maximum_attempts');
  if (attempt !== undefined) {
    parts.push(maximum === undefined ? t('productLogs.contextLine.attempt', { attempt }) : t('productLogs.contextLine.attemptOf', { attempt, maximum }));
  }
  const deleted = count('deleted');
  if (deleted !== undefined) parts.push(t('productLogs.contextLine.deleted', { count: deleted }));
  const retained = count('retained');
  if (retained !== undefined) parts.push(t('productLogs.contextLine.retained', { count: retained }));
  const assets = count('asset_count');
  if (assets !== undefined) parts.push(t('productLogs.contextLine.assets', { count: assets }));
  const freed = count('freed_bytes');
  if (freed !== undefined) parts.push(t('productLogs.contextLine.freed', { size: formatBytes(freed, options.language) }));
  const duration = count('duration_ms');
  if (duration !== undefined) parts.push(logDuration(duration));
  return parts.join(' · ');
}
