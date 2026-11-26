import type { ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Popover, PopoverContent, PopoverTrigger } from '@shared/components/ui/popover';

import type { SubscriptionSourcePreview } from '@/api/api-client';

import type { ReadIssue } from './source-read-issue';

import { contentLabel, contentProtocols } from './source-format';

/**
 * One line saying which nodes the content holds and the filter keeps, with an
 * action that lists them in a floating panel; or why they cannot be read, in
 * danger, with the step that may fix it in the action's place. The line keeps
 * its height in every state, so the form never moves.
 */
export function SourceNodeSummary({ issue, labelId, loading, onAction, preview, waiting }: {
  labelId: string;
  waiting: boolean;
  loading: boolean;
  /** Why the content, its fetch or the filter could not be read. */
  issue: ReadIssue | null;
  onAction: (action: NonNullable<ReadIssue['action']>) => void;
  preview: SubscriptionSourcePreview | undefined;
}) {
  const { t } = useTranslation();
  const problem = preview?.error;
  let status: ReactNode;
  if (waiting) {
    status = <span className='text-muted-foreground'>{t('subscriptions.sources.preview.waiting')}</span>;
  } else if (loading) {
    status = (
      <span className='flex items-center gap-2 text-muted-foreground'>
        <Spinner className='size-3.5' />
        {t('subscriptions.sources.preview.loading')}
      </span>
    );
  } else if (issue) {
    status = <span className='text-[var(--color-danger-text)]' title={issue.detail}>{issue.text}</span>;
  } else if (preview) {
    const label = contentLabel(preview.format, contentProtocols(preview.items), t);
    status = (
      <>
        <span className='tabular-nums'>
          {t('subscriptions.sources.preview.summary', { kept: preview.items.filter(item => item.kept).length, total: preview.items.length })}
        </span>
        {label
          ? (
              <>
                <span aria-hidden='true' className='mx-1.5 text-[var(--color-text-faint)]'>·</span>
                <span className='font-[family-name:var(--font-data)] text-xs text-[var(--color-text-secondary)]'>{label}</span>
              </>
            )
          : null}
      </>
    );
  }
  const items = loading || waiting || issue?.action ? [] : preview?.items ?? [];

  return (
    <div className='flex min-h-(--control-height) min-w-0 items-center gap-2'>
      <p aria-busy={loading || undefined} aria-labelledby={labelId} className='min-w-0 flex-1 truncate text-sm' role='status'>
        {status}
      </p>
      {issue?.action && !loading && !waiting
        ? (
            <Button className='flex-none text-primary' onClick={() => onAction(issue.action!)} size='sm' type='button' variant='ghost'>
              {t(issue.action === 'format' ? 'subscriptions.sources.actions.format' : 'subscriptions.sources.actions.retry')}
            </Button>
          )
        : null}
      {items.length > 0
        ? (
            <Popover>
              <PopoverTrigger render={<Button className='group/nodes flex-none gap-1 pe-2' size='sm' type='button' variant='ghost' />}>
                {t('subscriptions.sources.preview.view')}
                <ChevronDown
                  aria-hidden='true'
                  className='size-3.5 transition-transform duration-[var(--dur-short)] ease-[var(--ease-out)] group-data-popup-open/nodes:rotate-180 motion-reduce:transition-none'
                />
              </PopoverTrigger>
              <PopoverContent align='end' className='w-[min(28rem,calc(100vw-2rem))] gap-0 p-1' side='top' sideOffset={6}>
                <ScrollArea viewportClassName='max-h-60'>
                  <ul aria-label={t('subscriptions.sources.field.nodes')} className='font-[family-name:var(--font-data)] text-xs'>
                    {items.map(item => (
                      <li
                        className={cn(
                          'grid h-7 grid-cols-[minmax(0,1fr)_6rem_3rem] items-center gap-3 rounded-[var(--radius-control)] px-2',
                          !item.kept && 'text-muted-foreground',
                        )}
                        data-kept={item.kept}
                        key={item.location}
                        title={`${item.location} · ${item.name}`}
                      >
                        <span className={cn('truncate', item.location === problem?.location && 'text-[var(--color-danger-text)]')}>{item.name}</span>
                        <span className={cn('truncate', item.kept && 'text-[var(--color-text-secondary)]')}>{item.protocol}</span>
                        <span>{t(item.kept ? 'subscriptions.sources.preview.kept' : 'subscriptions.sources.preview.excluded')}</span>
                      </li>
                    ))}
                  </ul>
                </ScrollArea>
              </PopoverContent>
            </Popover>
          )
        : null}
    </div>
  );
}
