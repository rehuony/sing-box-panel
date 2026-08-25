import { ApiRequestError } from '@/api/api-client';

export interface ErrorNoticeProps {
  error: unknown;
  title?: string;
}

export function describeRequestError(error: unknown): string {
  if (error instanceof ApiRequestError) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return 'The request failed before the panel returned a response.';
}

export function ErrorNotice({
  error,
  title = 'This view could not be loaded',
}: ErrorNoticeProps) {
  return (
    <div className="notice notice--error" role="alert">
      <strong>{title}</strong>
      <p>{describeRequestError(error)}</p>
    </div>
  );
}
