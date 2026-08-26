import { describeRequestError } from './error-notice.utils';

export interface ErrorNoticeProps {
  error: unknown;
  title?: string;
}

export function ErrorNotice({
  error,
  title = 'This view could not be loaded',
}: ErrorNoticeProps) {
  return (
    <div className='notice notice--error' role='alert'>
      <strong>{title}</strong>
      <p>{describeRequestError(error)}</p>
    </div>
  );
}
