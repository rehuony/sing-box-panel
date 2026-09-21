import { useEffect, useId } from 'react';

import { toast } from '@/components/ui/toast-manager';

import { describeRequestError } from './error-notice.utils';

export interface ErrorNoticeProps {
  id?: string;
  error: unknown;
  title?: string;
}

export function ErrorNotice({ error, title, id }: ErrorNoticeProps) {
  const toastId = useId();
  const message = error == null || error === '' ? '' : describeRequestError(error);

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
