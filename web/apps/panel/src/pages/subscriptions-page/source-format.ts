import type { TFunction } from 'i18next';

import type { SubscriptionSource, SubscriptionSourceFormatChoice } from '@/api/api-client';

/**
 * The formats a source can name: auto, the configuration formats, then the
 * protocols share links convert to, for content holding only one of them.
 */
export const SOURCE_FORMATS = [
  'auto',
  'sing-box',
  'mihomo',
  'shadowsocks',
  'socks',
  'http',
  'vmess',
  'vless',
  'trojan',
  'hysteria',
  'hysteria2',
  'tuic',
  'anytls',
] as const satisfies readonly SubscriptionSourceFormatChoice[];

/** A format's name in a list: auto is described, a configuration format or protocol reads as written. */
export function formatChoiceLabel(format: SubscriptionSourceFormatChoice, t: TFunction): string {
  return format === 'auto' ? t('subscriptions.sources.formats.auto') : format;
}

/** The protocols of a content's nodes, in the order they first appear. */
export function contentProtocols(items: readonly { protocol: string }[]): string[] {
  return [...new Set(items.map(item => item.protocol))];
}

/**
 * What content holds: its configuration format, or for share links the
 * protocols they convert to; empty while those protocols are not known.
 */
export function contentLabel(format: SubscriptionSource['content_format'], protocols: readonly string[] | undefined, t: TFunction): string {
  if (format === undefined) return '';
  if (format !== 'share-links') return format;
  return protocols?.join(t('subscriptions.sources.protocolSeparator')) ?? '';
}
