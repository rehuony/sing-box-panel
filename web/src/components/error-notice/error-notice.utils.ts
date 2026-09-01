import i18n from '@/i18n';
import { ApiRequestError } from '@/api/api-client';

export function describeRequestError(error: unknown): string {
  if (error instanceof ApiRequestError) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return i18n.t('common.requestFailed');
}
