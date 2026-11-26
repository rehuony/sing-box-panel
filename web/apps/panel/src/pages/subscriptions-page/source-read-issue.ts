import type { TFunction } from 'i18next';

import type { SubscriptionSourceFormatChoice, SubscriptionSourcePreview } from '@/api/api-client';

import { ApiRequestError } from '@/api/api-client';
import { describeRequestError } from '@/components/error-notice';

type ParseError = NonNullable<SubscriptionSourcePreview['error']>;

/** Why a source cannot be read, said in the interface's language, and the step that may fix it. */
export interface ReadIssue {
  text: string;
  /** The server's own words, which name what it could not read; shown on hover. */
  detail?: string;
  action?: 'format' | 'retry';
}

const PARSE_CODES = new Set([
  'empty_content',
  'content_too_large',
  'unrecognized_format',
  'invalid_content',
  'too_many_nodes',
  'invalid_node_name',
  'node_not_convertible',
]);
const FETCH_PROBLEMS = new Set(['subscription_source_fetch_failed', 'subscription_source_url_rejected']);

/** A fetch failure's reason, as the server classifies it. */
export function fetchReason(reason: string | undefined, t: TFunction): ReadIssue {
  const status = /^http_status_(\d+)$/.exec(reason ?? '');
  if (status) return { action: 'retry', text: t('subscriptions.sources.fetchIssues.httpStatus', { status: status[1] }) };
  switch (reason) {
    case 'invalid_url':
    case 'address_denied':
    case 'body_too_large':
      return { text: t(`subscriptions.sources.fetchIssues.${reason}`) };
    case 'too_many_redirects':
    case 'request_failed':
      return { action: 'retry', text: t(`subscriptions.sources.fetchIssues.${reason}`) };
  }
  return { action: 'retry', text: t('subscriptions.sources.fetchIssues.unknown') };
}

/** Content the server could not read: the format it was read as decides what a fix may be. */
export function parseReason(problem: ParseError, format: SubscriptionSourceFormatChoice, t: TFunction): ReadIssue {
  const values = { format, location: problem.location ?? '', name: problem.name ?? '' };
  const detail = problem.reason;
  switch (problem.code) {
    case 'unrecognized_format':
      return { action: 'format', text: t('subscriptions.sources.readIssues.unrecognized_format') };
    case 'invalid_content': {
      let key = 'invalid_content';
      if (format !== 'auto') key = 'invalid_format';
      else if (problem.location) key = 'invalid_content_at';
      return { action: 'format', detail, text: t(`subscriptions.sources.readIssues.${key}`, values) };
    }
    case undefined:
      break;
    default:
      if (PARSE_CODES.has(problem.code)) return { detail, text: t(`subscriptions.sources.readIssues.${problem.code}`, values) };
  }
  return { detail, text: t('subscriptions.sources.readIssues.unknown', { code: problem.code }) };
}

/** A request that failed: a fetch the server tried, or anything else in its own words. */
export function requestReason(error: unknown, t: TFunction): ReadIssue {
  if (error instanceof ApiRequestError && error.code && FETCH_PROBLEMS.has(error.code)) {
    return fetchReason(error.fields?.reason, t);
  }
  return { action: 'retry', text: describeRequestError(error) };
}

/** A source's last failed attempt, by the code it recorded: a fetch failure or content it could not read. */
export function attemptReason(code: string, t: TFunction): string {
  if (PARSE_CODES.has(code)) return parseReason({ code }, 'auto', t).text;
  return fetchReason(code, t).text;
}
