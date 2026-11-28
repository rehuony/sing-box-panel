import { useEffect, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from '@shared/components/ui/toast-manager';

import { describeRequestError } from './error-notice.utils';

export interface ErrorNoticeProps {
  id?: string;
  error: unknown;
  title?: string;
}

export function ErrorNotice({ error, title: given, id }: ErrorNoticeProps) {
  const { t } = useTranslation();
  const toastId = useId();
  const message = error == null || error === '' ? '' : describeRequestError(error);
  // A message the page wrote is short enough to title itself; a request's reason goes under a short title.
  const title = given ?? (typeof error === 'string' ? undefined : t('common.requestFailedTitle'));

  useEffect(() => {
    if (!message) return;
    let cancelled = false;
    // Let the ancestor Toaster subscribe before reporting an error on initial mount.
    queueMicrotask(() => {
      if (cancelled) return;
      toast.add({
        id: toastId,
        title: title || message,
        description: title && title !== message ? message : undefined,
        type: 'error',
        priority: 'high',
      });
    });
    return () => {
      cancelled = true;
      toast.close(toastId);
    };
  }, [message, title, toastId]);

  // Preserve field descriptions for assistive technology without occupying layout space.
  return id && message ? <span id={id} className='sr-only'>{message}</span> : null;
}
