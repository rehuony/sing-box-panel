import i18n from '@/i18n';
import { ApiRequestError } from '@/api/api-client';

/**
 * A problem the panel reported by code, in the interface's language when the
 * locales know the code, and otherwise as the panel's own detail.
 */
export function describeProblem(code: string | undefined, detail: string): string {
  return code && i18n.exists(`productLogs.errors.${code}`) ? i18n.t(`productLogs.errors.${code}`) : detail;
}

export function describeRequestError(error: unknown): string {
  if (typeof error === 'string') return error;
  if (error instanceof ApiRequestError) {
    if (error.code === 'configuration_not_saved') return i18n.t('cores.error.configurationNotSaved');
    if (error.code === 'subscription_token_secret_unavailable') return i18n.t('subscriptions.keys.secretUnavailable');
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return i18n.t('common.requestFailed');
}
