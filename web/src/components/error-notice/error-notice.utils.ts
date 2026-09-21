import i18n from '@/i18n';
import { ApiRequestError } from '@/api/api-client';

export function describeRequestError(error: unknown): string {
  if (typeof error === 'string') return error;
  if (error instanceof ApiRequestError) {
    if (error.code === 'subscription_token_secret_unavailable') return i18n.t('subscriptions.keys.secretUnavailable');
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return i18n.t('common.requestFailed');
}
