import { ApiRequestError } from '@/api/api-client';

export function describeRequestError(error: unknown): string {
  if (error instanceof ApiRequestError) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return 'The request failed before the panel returned a response.';
}
