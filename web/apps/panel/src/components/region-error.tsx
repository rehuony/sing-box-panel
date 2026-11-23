import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';

import { describeRequestError } from '@/components/error-notice';

/** A region whose data could not be read: what failed, why, and a retry, centred in place of an empty region. */
export function RegionError({ error, onRetry, title }: { error: unknown; onRetry: () => void; title: string }) {
  const { t } = useTranslation();
  return (
    <div className='flex h-full min-h-48 flex-1 flex-col items-center justify-center gap-3 px-4 text-center' role='status'>
      <h2 className='m-0 text-sm font-medium'>{title}</h2>
      <p className='m-0 max-w-md text-sm text-muted-foreground wrap-anywhere'>{describeRequestError(error)}</p>
      <Button onClick={onRetry} type='button' variant='outline'>{t('shell.error.retry')}</Button>
    </div>
  );
}
