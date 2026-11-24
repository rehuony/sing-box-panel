import type { ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';

import { cn } from '@shared/lib/utils';
import { CircleAlert } from 'lucide-react';
import { useEffect, useEffectEvent } from 'react';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import { usePageVisible } from '@/hooks/use-page-visible';
import { describeRequestError } from '@/components/error-notice';

/** How often a region whose data could not be read reads it again while the page is in view. */
const RELOAD_INTERVAL_MS = 10_000;

/**
 * What a data region shows in place of its items: an icon and a title, with
 * a reason below it when there is one, centred in the region. It offers no
 * action; adding, refreshing and filtering stay in the region's toolbar.
 */
export function RegionState({ className, description, icon: Icon, title }: {
  icon: LucideIcon;
  title: ReactNode;
  className?: string;
  description?: ReactNode;
}) {
  return (
    <Empty className={cn('min-h-40', className)} role='status'>
      <EmptyHeader>
        <EmptyMedia aria-hidden='true' variant='icon'><Icon /></EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        {description ? <EmptyDescription className='wrap-anywhere'>{description}</EmptyDescription> : null}
      </EmptyHeader>
    </Empty>
  );
}

/**
 * A region whose data could not be read: what failed and why. While it shows
 * and the page is in view, it reads the region again on its own.
 */
export function RegionError({ className, error, reload, title }: {
  title: string;
  error: unknown;
  reload: () => void;
  className?: string;
}) {
  const visible = usePageVisible();
  const read = useEffectEvent(reload);
  useEffect(() => {
    if (!visible) return;
    const timer = setInterval(read, RELOAD_INTERVAL_MS);
    return () => clearInterval(timer);
  }, [visible]);
  return (
    <RegionState className={className} description={describeRequestError(error)} icon={CircleAlert} title={title} />
  );
}
