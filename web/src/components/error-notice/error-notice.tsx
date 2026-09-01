import { useTranslation } from 'react-i18next';

import { describeRequestError } from './error-notice.utils';

export interface ErrorNoticeProps {
  error: unknown;
  title?: string;
}

export function ErrorNotice({
  error,
  title,
}: ErrorNoticeProps) {
  const { t } = useTranslation();

  return (
    <div className='notice notice--error' role='alert'>
      <strong>{title ?? t('common.viewLoadFailed')}</strong>
      <p>{describeRequestError(error)}</p>
    </div>
  );
}
